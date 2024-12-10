FROM ubuntu:latest
LABEL authors="christophebuffard"

EXPOSE 8090

WORKDIR /app
COPY oem-bridge ./
COPY config.yaml ./
CMD ["/app/oem-bridge"]