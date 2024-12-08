# RabbitMQ conf and usefully command

## Command
```shell
rabbitmq-diagnostics listeners
rabbitmq-plugins enable rabbitmq_management
rabbitmqctl eval "application:get_env(rabbit, auth_mechanisms)."
```

## Server CA certifile

The ssl_options.cacertfile in the RabbitMQ configuration must include the CA certificate that signed the client certificate.

1.	Locate the CA certificate that issued the client certificate (e.g., client_ca.pem).
2.	Update the rabbitmq.conf to point to this CA certificate:

```shell
ssl_options.cacertfile = /path/to/client_ca.pem
```

3. If your CA certificate chain includes intermediates, ensure the cacertfile includes the full chain (root + intermediates) in PEM format.

Example CA chain file (client_ca_chain.pem):
```shell
-----BEGIN CERTIFICATE-----
[Root CA certificate]
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
[Intermediate CA certificate]
-----END CERTIFICATE-----
```
### Steps to Configure RabbitMQ for Multiple Clients Using a Private CA

1. Consolidate the CA Certificate Chain
- Gather all certificates in the private CA chain, including:
- The root CA certificate. 
- Any intermediate CA certificates used to sign the client certificates. 
- Create a single CA bundle file (e.g., private_ca_chain.pem) by concatenating the certificates in the correct order:

```shell
cat root_ca.pem intermediate_ca.pem > /etc/rabbitmq/private_ca_chain.pem
```
2. Update RabbitMQ Configuration
   •	Edit the RabbitMQ configuration file (rabbitmq.conf) to include the private CA bundle for validating client certificates.

```editorconfig
# SSL/TLS options
ssl_options.cacertfile = /etc/rabbitmq/private_ca_chain.pem
ssl_options.certfile   = /etc/rabbitmq/certs/rabbitmq_server.pem
ssl_options.keyfile    = /etc/rabbitmq/certs/rabbitmq_server.key
ssl_options.verify     = verify_peer
ssl_options.fail_if_no_peer_cert = true

# Authentication mechanism for client certificates
auth_mechanisms.1 = EXTERNAL
```
- Key Directives:
    - ssl_options.cacertfile: Points to the CA chain file containing the trusted private CA certificates. 
    - ssl_options.certfile: The RabbitMQ server’s certificate (signed by the private CA). 
    - ssl_options.keyfile: The RabbitMQ server’s private key. 
    - ssl_options.verify = verify_peer: Ensures RabbitMQ validates client certificates. 
    - ssl_options.fail_if_no_peer_cert = true: Rejects connections from clients that do not present a valid certificate. 
    - auth_mechanisms.1 = EXTERNAL: Enables authentication using the client certificate.

In RabbitMQ, when using the EXTERNAL authentication mechanism, the server derives the client’s username from the client’s 
TLS (x.509) certificate. By default, RabbitMQ extracts the username from the certificate’s Distinguished Name (DN). However, 
you can configure RabbitMQ to extract the username from the certificate’s Common Name (CN) by setting the ssl_cert_login_from 
configuration option to common_name.
