FROM scratch
MAINTAINER Brian Akins <brian@akins.org>
COPY configmap-aggregator.linux /configmap-aggregator
ENTRYPOINT [ "/configmap-aggregator" ]
