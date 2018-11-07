FROM alpine:3.8

RUN mkdir -p /opt
ADD ./k8s-endpoint-updater /opt/k8s-endpoint-updater

ENTRYPOINT ["/opt/k8s-endpoint-updater"]
