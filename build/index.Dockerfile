FROM quay.io/operator-framework/upstream-registry-builder as builder

# 9.9.9 points to the templated manifests for CI
ARG OPERATOR_VERSION_NEXT=9.9.9

RUN mkdir -p /tmp/manifests
COPY manifests/node-maintenance-operator/v"${OPERATOR_VERSION_NEXT}" /tmp/manifests/

# Build index database
RUN mkdir -p /tmp/database
RUN /bin/initializer -m /tmp/manifests -o /tmp/database/index.db

FROM quay.io/operator-framework/upstream-opm-builder

COPY --from=builder /tmp/database/index.db /database/index.db

EXPOSE 50051
ENTRYPOINT ["/bin/opm"]
CMD ["registry", "serve", "--database", "/database/index.db"]

LABEL operators.operatorframework.io.index.database.v1=/database/index.db
