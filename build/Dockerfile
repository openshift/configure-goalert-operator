FROM quay.io/redhat-services-prod/openshift/boilerplate:image-v8.3.6 AS builder

WORKDIR /workdir
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY . .
RUN make go-build

####
FROM registry.access.redhat.com/ubi9/ubi-minimal:1786380947

ENV USER_UID=1001 \
    USER_NAME=configure-goalert-operator

COPY --from=builder /workdir/build/_output/bin/* /usr/local/bin/

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

LABEL io.openshift.managed.name="configure-goalert-operator" \
      io.openshift.managed.description="Operator to manage registering clusters with GoAlert" \
      MAINTAINERS="ROSA GovCloud SREP Team (dfreiber-all)" \
      BUILD_REPO="https://github.com/openshift/configure-goalert-operator" \
      version="v3.0.0"
