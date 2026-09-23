# `.opencode/` agent tooling — trust model & monitoring

## What lives here
- `package.json` / `package-lock.json` — pinned agent plugin dependency `@opencode-ai/plugin`
  (currently 1.18.18), all packages from `registry.npmjs.org` with SRI integrity hashes.
- `node_modules/` — installed tree (gitignored); `plans/` — tracked planning docs (no secrets).

## Trust model (OS4)
The plugin executes with full developer privileges (spawns processes, reads the repo and local
git/SSH/API-key material). A compromised plugin is therefore equivalent to workstation compromise.
Treat every `@opencode-ai/*` upgrade as a privileged dependency change:
- Review the diff and provenance before upgrading.
- Prefer upgrades proposed via Dependabot + review; never auto-merge.

## Monitoring (OS1, OS3)
- **OS1 — beta dependency:** `effect@4.0.0-beta.83` is pinned by the plugin and frozen by the lockfile.
  Betas are repointable upstream. Do NOT bump without a full `npm ci && npm audit && npm audit
  signatures` re-run and a review of the new tree.
- **OS3 — native postinstall:** `msgpackr-extract` (`hasInstallScript: true`) downloads platform
  prebuilt binaries during install. It is integrity-pinned. If a stricter stance is required, install
  with `--ignore-scripts` (may degrade functionality) or pre-vet the prebuilt artifacts.
- CI runs `npm ci && npm audit && npm audit signatures` on every PR and push (see
  `.github/workflows/*.yaml`). Keep that job green; a red audit blocks merge.
