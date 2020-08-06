FROM quay.io/operator-framework/upstream-registry-builder as builder
ARG OPERATOR_VERSION_NEXT
WORKDIR /sources/
COPY . .
RUN mkdir database && /bin/initializer -m /sources/manifests/node-maintenance-operator/v"${OPERATOR_VERSION_NEXT}" -o database/index.db

FROM quay.io/operator-framework/upstream-opm-builder

COPY --from=builder /sources/database/index.db /database/index.db

EXPOSE 50051
ENTRYPOINT ["/bin/opm"]
CMD ["registry", "serve", "--database", "/database/index.db"]

LABEL operators.operatorframework.io.index.database.v1=/database/index.db
