FROM quay.io/operator-framework/upstream-registry-builder as builder

# 9.9.9 points to the templated manifests for CI
ARG OPERATOR_VERSION_NEXT=9.9.9
ARG OPENSHIFT_BUILD_NAMESPACE
ENV REG_URL=registry.svc.ci.openshift.org

WORKDIR /manifests/
COPY manifests/node-maintenance-operator/v"${OPERATOR_VERSION_NEXT}" /manifests/

# Replace template vars when in CI
RUN find /manifests/ -type f -exec sed -i "s|IMAGE_REGISTRY/OPERATOR_IMAGE:IMAGE_TAG|${REG_URL}/${OPENSHIFT_BUILD_NAMESPACE}/stable:node-maintenance-operator|g" {} \; || :

# Build index database
RUN mkdir /database && /bin/initializer -m /manifests -o /database/index.db

FROM quay.io/operator-framework/upstream-opm-builder

COPY --from=builder /database/index.db /database/index.db

EXPOSE 50051
ENTRYPOINT ["/bin/opm"]
CMD ["registry", "serve", "--database", "/database/index.db"]

LABEL operators.operatorframework.io.index.database.v1=/database/index.db
