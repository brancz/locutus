package resource

import (
	"regexp"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"
)

type TransformationAction = string

const (
	KeyReplace TransformationAction = "replace"
	KeyDrop    TransformationAction = "drop"
	KeyKeep    TransformationAction = "keep"
)

type keyTransformation struct {
	action      TransformationAction
	regex       *regexp.Regexp
	replacement string
}

func newKeyTransformation(keyTransformationConfig KeyTransformationConfig) (*keyTransformation, error) {
	actionString := keyTransformationConfig.Action
	if actionString == "" {
		actionString = "replace"
	}

	regexString := keyTransformationConfig.Regex
	if regexString == "" {
		regexString = "(.*)"
	}

	regex, err := regexp.Compile(regexString)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compile key transformation's regex")
	}

	replacement := keyTransformationConfig.Replacement
	if replacement == "" {
		replacement = "$1"
	}

	return &keyTransformation{
		action:      actionString,
		regex:       regex,
		replacement: replacement,
	}, nil
}

func (t *keyTransformation) transform(key string) string {
	switch t.action {
	case KeyReplace:
		indices := t.regex.FindStringSubmatchIndex(key)
		if indices == nil {
			return key
		}

		return string(t.regex.ExpandString([]byte{}, t.replacement, key, indices))
	case KeyDrop:
		if !t.regex.MatchString(key) {
			return ""
		}
		return key
	case KeyKeep:
		if t.regex.MatchString(key) {
			return key
		}
		return ""
	}

	return key
}

type keyTransformations []*keyTransformation

func (t keyTransformations) transform(key string) string {
	cur := key
	for _, keyTransformation := range t {
		cur = keyTransformation.transform(cur)
	}

	return cur
}

type ResourceHandlers struct {
	inf                cache.SharedIndexInformer
	keyTransformations keyTransformations
	enqueueFunc        func(obj interface{})
	keyFunc            func(obj interface{}) (string, bool)

	logger log.Logger
}

func NewResourceHandlers(logger log.Logger, inf cache.SharedIndexInformer, enqueueFunc func(obj interface{}), keyFunc func(obj interface{}) (string, bool), keyTransformationConfigs []KeyTransformationConfig) (*ResourceHandlers, error) {
	keyTransformations := keyTransformations{}
	for _, keyTransformationConfig := range keyTransformationConfigs {
		keyTransformation, err := newKeyTransformation(keyTransformationConfig)
		if err != nil {
			return nil, err
		}

		keyTransformations = append(keyTransformations, keyTransformation)
	}

	return &ResourceHandlers{
		logger:             logger,
		inf:                inf,
		keyTransformations: keyTransformations,
		enqueueFunc:        enqueueFunc,
		keyFunc:            keyFunc,
	}, nil
}

func (r *ResourceHandlers) OnAdd(obj interface{}, isInInitialList bool) {
	key, ok := r.keyFunc(obj)
	if !ok {
		return
	}
	level.Debug(r.logger).Log("action", "add", "key", key)

	newKey := r.keyTransformations.transform(key)
	level.Debug(r.logger).Log("msg", "transformed key", "original-key", key, "new-key", newKey)
	if newKey == "" {
		level.Debug(r.logger).Log("msg", "dropping handler as key was either dropped or empty")
		return
	}

	r.enqueueFunc(newKey)
}

func (r *ResourceHandlers) OnDelete(obj interface{}) {
	key, ok := r.keyFunc(obj)
	if !ok {
		return
	}
	level.Debug(r.logger).Log("action", "delete", "key", key)

	newKey := r.keyTransformations.transform(key)
	level.Debug(r.logger).Log("msg", "transformed key", "original-key", key, "new-key", newKey)
	if newKey == "" {
		level.Debug(r.logger).Log("msg", "dropping handler as key was either dropped or empty")
		return
	}

	r.enqueueFunc(newKey)
}

func (r *ResourceHandlers) OnUpdate(old, cur interface{}) {
	curKey, ok := r.keyFunc(cur)
	if !ok {
		return
	}

	oldAccessor, oldErr := meta.CommonAccessor(old)
	curAccessor, curErr := meta.CommonAccessor(cur)
	if old != nil && oldErr == nil && cur != nil && curErr == nil {
		if oldAccessor.GetResourceVersion() == curAccessor.GetResourceVersion() {
			level.Debug(r.logger).Log("msg", "resource version unchanged", "key", curKey)
			return
		}
	}
	level.Debug(r.logger).Log("action", "update", "key", curKey)

	newKey := r.keyTransformations.transform(curKey)
	level.Debug(r.logger).Log("msg", "transformed key", "original-key", curKey, "new-key", newKey)
	if newKey == "" {
		level.Debug(r.logger).Log("msg", "dropping handler as key was either dropped or empty")
		return
	}

	r.enqueueFunc(newKey)
}
