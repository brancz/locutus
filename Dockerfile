FROM gcr.io/distroless/static

COPY locutus /locutus

WORKDIR /
ENTRYPOINT [ "/locutus" ]
