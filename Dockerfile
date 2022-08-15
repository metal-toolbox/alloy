FROM alpine:latest

ENTRYPOINT ["/usr/sbin/alloy"]

COPY alloy-linux /usr/sbin/alloy
RUN chmod +x /usr/sbin/alloy
