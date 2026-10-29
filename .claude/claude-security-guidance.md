# Security rules — HTTPaaS

This app runs on the host, executes VBoxManage and remote commands over SSH, and accepts uploaded zip files.

## Must never happen
- User input (hostname, file names, zip entry names) concatenated into a shell string, `sh -c`, `bash -c`, or a heredoc.
  Use `exec.CommandContext(name, args...)` with validated args, or fixed scripts that receive `$1`, `$2`.
- Running any command before `dominio.ValidarHostname` (regex `^[a-z][a-z0-9-]{1,30}[a-z0-9]$`, reserved names rejected)
  and IP range validation.
- Extracting or forwarding a zip that was not checked by `publicar.ValidarZip`: absolute paths, `..`, symlinks,
  >2000 entries, >200 MB uncompressed, or missing `index.html` must be rejected.
- Reading an upload without `http.MaxBytesReader`.
- POST handlers accepting requests whose `Origin` is not the app itself.
- SSH without `BatchMode=yes`, or with `StrictHostKeyChecking=no`. Use `accept-new` plus the app's own known_hosts.
- Broad sudo: the `httpaas` user may only run `/usr/local/sbin/httpaas-dns` (on ns) or `/usr/local/sbin/httpaas-publicar`
  (on instances), and those scripts re-validate their arguments.
- Secrets in the repo, logs or HTML: private keys, `config.json`, passwords. `keys/`, `data/` and `logs/` are gitignored.
- Rendering user data with `text/template` or string concatenation into HTML. Use `html/template`.
- Writing `data/instancias.json` in place: always temp file + rename.
- Applying Bind9/Apache/sshd/sudoers config without `named-checkconf`/`named-checkzone`, `apache2ctl configtest`, `sshd -t`, `visudo -cf` first.
