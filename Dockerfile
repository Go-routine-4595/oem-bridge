FROM ubuntu:latest
LABEL authors="christophebuffard"

EXPOSE 8090

WORKDIR /app
COPY oem-bridge-linux-arm64 ./
COPY config2.yaml ./
CMD ["/app/oem-bridge-linux-arm64", "/app/config2.yaml"]