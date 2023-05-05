BEGIN;

CREATE TABLE IF NOT EXISTS test
(
   id UUID NOT NULL DEFAULT gen_random_uuid(),
   name VARCHAR (50) NOT NULL,
   enabled BOOLEAN NOT NULL,

   PRIMARY KEY (id)
);

COMMIT;
