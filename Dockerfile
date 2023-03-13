FROM alpine:3.17.2

ENTRYPOINT ["/usr/sbin/alloy"]

COPY alloy /usr/sbin/alloy
RUN chmod +x /usr/sbin/alloy
