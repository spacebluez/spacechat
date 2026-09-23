# TLS certificates

Place both `tls.crt` (the complete PEM certificate chain) and `tls.key`
(the matching private key) in this directory before starting Compose.
The certificate SAN must match the hostname or IP used by clients.

With neither file present, the server uses plaintext HTTP/WS. The default
network allowlist accepts all IPv4 and IPv6 clients, including public IPs.
With just one file, or a dangling certificate/key symlink, startup fails.
Restart the server after replacing certificates. These files are ignored by
Git and excluded from the Docker build context.

The directory is mounted read-only into the server. The entrypoint copies
the pair into its private runtime directory before dropping to UID 10001;
the cleanup service cannot access that directory. Keep the host private
key readable only by its owner. Include any symlink targets inside the
mounted directory, or provide regular files.

For an internal CA, point `SPACECHAT_CLIENT_CA_FILE` at the public CA PEM
file so the artifacts service can embed it in clients. Never use `tls.key`
as the client CA file. Set `SPACECHAT_PUBLIC_URL=wss://hostname:18081/ws`
when clients need a fixed public address.
