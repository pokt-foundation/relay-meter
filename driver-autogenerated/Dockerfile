# This Dockerfile used to build the image used for testing TxDB
FROM postgres:14.3

COPY ./sqlc/schema.sql /docker-entrypoint-initdb.d/
