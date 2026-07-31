# Linux VPS distribution — research & proposal

**Status:** proposal, not implemented. Nothing in this document has been built yet.
**Target environment (confirmed):** Ubuntu/Debian VPS, binary installed directly on the host (no container).
**Upgrade model (confirmed):** upgrades are performed **manually by a human**, not by an unattended timer or `apt upgrade`. This settles §4.2 and rules out option C — see §3.
**Goal:** a first-class way to install *and keep updating* `seli` on a Linux VPS, the way `brew install --cask` serves macOS today.

---

## 1. What we ship today

| Channel | Config | Works on an Ubuntu VPS? |
|---|---|---|
| GitHub Releases tarballs | `.goreleaser.yml` → `archives`, `goos: linux`, `goarch: [amd64, arm64]` | Yes, but manual: download, untar, `mv` into PATH, no upgrade path |
| `go install` | — | Only if Go is installed on the VPS; also lands as `seli-api-sdk` and must be renamed |
| Build from source | `make build` | Requires the Go toolchain on the VPS |
| Homebrew **cask** | `.goreleaser.yml` → `homebrew_casks` | **No.** Casks are macOS-only; `brew install --cask` does not work on Linuxbrew |

So the Linux binaries already exist and are already signed into `checksums.txt` on every release — what is missing is not the *artifact*, it is the *install and upgrade path*.

### Bug found while researching

`README.md` heads the Homebrew section **"Homebrew (macOS / Linux)"**. That is wrong: the tap ships a cask, and casks do not install on Linux. Anyone following the README on a VPS hits a hard failure. This should be corrected in whichever PR implements the proposal (or sooner, as a standalone doc fix).

---

## 2. Options considered

### A. Install script (`curl … | sh`)

A `scripts/install.sh` in the repo, served from a stable URL, that detects OS/arch, resolves the latest release tag from the GitHub API (or takes `SELI_VERSION`), downloads the matching tarball, **verifies it against `checksums.txt`**, and installs the binary to `/usr/local/bin/seli`.

- **Cost:** low — one script, no new CI secrets, no hosting beyond raw GitHub (or a `get.seli.app` redirect later).
- **Upgrade:** re-run the script. Good enough; not integrated with the system package manager.
- **Reach:** every distro, not just Debian — covers us if the VPS fleet ever diversifies.
- **Risk:** `curl | sh` is a supply-chain-shaped pattern. Mitigations: checksum verification inside the script, and document a two-step form (`curl -O` → inspect → `sh`) alongside the one-liner. Never pipe a script that we don't check the checksum inside of.

### B. `.deb` package via GoReleaser `nfpms`

GoReleaser builds `.deb` from the existing binaries with a small `nfpms:` block (verified against current GoReleaser docs — the section takes `formats: [deb]`, `bindir`, `maintainer`, `license`, `homepage`, plus optional `deb.signature` for a GPG-signed package). Artifacts land on the GitHub Release next to the tarballs automatically.

- **Cost:** low — roughly 15 lines in `.goreleaser.yml`, no new workflow steps.
- **Install:** `apt install ./seli_<ver>_linux_amd64.deb` after downloading. Files land in the right place, `dpkg -l` knows about it, uninstall is clean.
- **Upgrade:** still manual (download the new `.deb`, install it) **unless** we also host an apt repository — see C.

### C. An actual apt repository (`apt update && apt upgrade` works)

This is the only option that gives true package-manager-native upgrades. Two realistic routes:

1. **Gemfury** — GoReleaser has first-class support (`gemfury:` publisher, `FURY_TOKEN` env var). It hosts the apt repo for us. Cost: a Gemfury account (paid for private repos) + one new CI secret.
2. **Self-hosted on GitHub Pages** — generate the repo index (`aptly` / `reprepro`) in CI, sign with a GPG key, publish to Pages. Cost: a GPG signing key in CI secrets, key-distribution to every VPS, and an index-generation step we own and maintain forever.

Either way this is meaningfully more work than A or B, and it introduces a **key-management** obligation (an apt repo without a signing key is a repo `apt` will refuse or warn on).

### D. Docker image — rejected for this target

The user confirmed direct-on-host installs, and `seli` is a CLI that an agent invokes via `shell exec`. Wrapping it in a container adds a layer between the agent and the binary for no benefit here. Worth revisiting only if the VPS fleet becomes container-only.

### E. Resurrect the Homebrew **formula** (Linuxbrew) — rejected

A formula *would* install on Linuxbrew and give `brew upgrade` on Linux. But the release workflow currently **deletes `seli.rb` from the tap on every release** ("Cask is canonical"), precisely because a formula and cask of the same name in the same tap collided before and forced users through `brew uninstall --formula`. Re-adding a formula walks straight back into that. Also, requiring Homebrew on a VPS is a heavier dependency than a `.deb`.

---

## 3. Recommendation

**Ship A + B. Option C is rejected.**

The only thing an apt repository buys over a plain `.deb` is `apt upgrade` — i.e. package-manager-driven upgrades. The upgrade model is confirmed as *manual, by a human*, so that benefit is worth nothing here, while the cost (hosting the repo index forever, a GPG signing key in CI, distributing that key to every VPS) is permanent. Re-run the install script, or `apt install` the new `.deb`.

- **A (install script)** is the headline install path for a VPS and covers non-Debian hosts.
- **B (`.deb`)** costs almost nothing on top of what GoReleaser already builds, and makes the box's package manager aware of the binary (clean install/uninstall, `dpkg -l seli`).
- **C (apt repo)** is dropped, per the confirmed manual-upgrade model above.

Neither A nor B touches the binary, the command surface, or the output contract — so per the repo's rule, neither requires a skill change. **What may require a skill/doc change is the VPS *bootstrap* story**, below.

---

## 4. Open questions for the VPS bootstrap (not just "how do we get the bytes there")

Installing the binary is the easy half. For an agent host, two more things must be answered before anyone calls this done:

1. **Unattended auth.** `seli auth login --key` writes `~/.seli/config.json` interactively. The CLI already honours **`SELI_API_KEY`** (and `SELI_TENANT`, `SELI_API_URL`), so a VPS can be provisioned with no interactive login at all. Whichever install path we ship should document the env-var route as the *first-class* one for a headless box, not the interactive login.
2. **Who upgrades, and when.** ~~Open.~~ **Settled: a human, manually** — the same shape as `brew upgrade` on `vtech:tam` today. This is what kills option C. The install script must therefore be safely re-runnable over an existing install (idempotent: overwrite the binary in place, don't append to PATH twice, don't fail if `/usr/local/bin/seli` already exists).

---

## 5. Proposed scope if approved

1. `.goreleaser.yml`: add an `nfpms` block producing `deb` for linux amd64/arm64.
2. `scripts/install.sh`: OS/arch detection, latest-or-pinned version, checksum verification, install to `/usr/local/bin`.
3. `README.md`: new "Linux (VPS)" install section; **fix the incorrect "Homebrew (macOS / Linux)" heading**; document `SELI_API_KEY`-based headless setup.
4. `CHANGELOG.md`: `[Unreleased]` entry.
5. Verification: cut a snapshot build (`goreleaser release --snapshot --clean`), install the resulting `.deb` and run the script end-to-end on a real Ubuntu box (or a Lima VM) — not just "CI is green".
