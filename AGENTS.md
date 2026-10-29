# Instructions for AI assistants

Project: HTTPaaS (Cloud Computing, partial exam 3). A Go web app on the host that provisions Apache VMs
from a VirtualBox multiattach disk, registers them in Bind9 (`cloud.local`) and deploys a user `.zip`.

## Read first
1. `docs/PLAN.md` — architecture (§2), decisions D1–D16 (§3), shared contract (§4), rules (§8).
2. The plan of the part you are helping with: `docs/parte-a/PLAN.md` (Cristhian), `docs/parte-b/PLAN.md` (Daniel)
   or `docs/parte-c/PLAN.md` (Wiliam). Its "Para el asistente de IA" block is binding.

## Hard rules
- Work one phase at a time, in order. A phase is done only when its "Verificación" passes against the real VM.
- Only edit the folders owned by the current part (`docs/PLAN.md` §4.1). The shared contract (§4) changes only by team agreement.
- Never build shell commands from user data. Use `exec.CommandContext(name, args...)` with validated args; never `sh -c`.
- Validate every input before executing anything; invalid input → HTTP 422 and nothing runs.
- Pure functions for anything that generates config or arguments, with table-driven tests. Use the Fakes in `internal/puertos`.
- `make check` must pass before every commit. Conventional commits in English that state what changed.
- Never commit `keys/`, `data/`, `logs/`, `config.json`, private keys or disk images.
- Do not invent VBoxManage, Bind9 or Apache flags: check `--help`, man pages or official docs.
- Language: repository docs in Spanish. Package names, types, functions and fields use the Spanish names defined
  in the shared contract (`docs/PLAN.md` §4: `puertos`, `Hipervisor`, `ValidarHostname`, `SolicitarCrear`…) and new
  identifiers follow that same style. Code comments and commit messages in English.
- On Windows there is no `make`: use the equivalent commands in `docs/PLAN.md` §7 (P5). Shell scripts must keep LF line endings.
