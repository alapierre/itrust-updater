[![Sonarcloud Status](https://sonarcloud.io/api/project_badges/measure?project=alapierre_itrust-updater&metric=alert_status)](https://sonarcloud.io/dashboard?id=alapierre_itrust-updater)
[![Build](https://github.com/alapierre/itrust-updater/actions/workflows/build.yml/badge.svg)](https://github.com/alapierre/itrust-updater/actions/workflows/build.yml)

# itrust-updater

Secure application updater for binary artifacts. Supports Ed25519 manifest signing, SHA256 artifact verification, and multiple repository management.

`itrust-updater` was created to solve a recurring problem of securely delivering binary application artifacts to end users and customers.
Many projects need to distribute executables such as Go-based binaries (.exe) or Java applications (.jar) across multiple platforms, primarily Windows and Linux, in a way that is both secure and easy to operate.

The tool focuses on secure artifact distribution by ensuring file integrity and authenticity — verifying that an artifact has not been tampered with and that it originates from a trusted publisher. At the same time, it aims to remain simple to use for both publishers and clients, without requiring complex infrastructure or manual steps.

itrust-updater is designed to integrate naturally with CI/CD pipelines, enabling automated publishing, updating, and verification of artifacts, while providing a consistent update mechanism for end users across platforms.

## Key Concepts

- **Profile**: A configuration for a specific application on a machine (e.g., `go-ksef`).
- **Repository (Repo)**: A backend storage (Nexus RAW) containing artifacts and manifests. Identified by `repo-id`.
- **Channel**: Release channel (e.g., `stable`, `beta`).
- **Security**: Mandatory signature verification and public key pinning (fingerprint verification).
- **Artifact Types**: Supports `binary` (OS/Arch specific) and `jar` (multi-platform, OS/Arch="any").

## CLI Commands

### Repository Management (`repo`)

Manage repository configurations and secrets.

- **`repo init --repo-id <id> --base-url <url> --nexus-user <user>`**:
  Initializes a new repository. Generates a new Ed25519 signing key, uploads the public key to the repo, and saves local configuration.
  Use `--use-keyring` to store the generated seed and Nexus credentials securely.
  With keyring enabled, the seed and credentials are saved and read back before the public key is published. A storage or verification failure aborts initialization. Failed uploads or configuration writes leave the seed in the keyring; retrying reuses it and can finish initialization if the same public key was already uploaded. Initialization refuses to replace a different remote public key or an existing local repository configuration.
  Without keyring, the seed is printed **before** publication; capture it securely, including if a later step fails. A failed output prevents publication. Local initializations for the same repository ID are serialized; after a crash, an error identifies the `.init-lock` directory to remove only when no initialization is running.
  Repository credentials must allow checking whether the public key exists and reading it when present, in addition to uploading a new key.
- **`repo config --repo-id <id>`**:
  Displays a configuration snippet for the given repository (URL, public key path, and fingerprint).
- **`repo export --repo-id <id> [--include-seed] [--include-nexus] [--out <file>]`**:
  Creates a bundle (env format) to migrate repository configuration to another machine. Can include the signing seed and Nexus credentials.
- **`repo import [--in <file>] [--write-repo-config]`**:
  Imports repository configuration and secrets from an exported bundle.

### Application Management

- **`init <profile> --app-id <id> --base-url <url> --repo-pubkey-sha256 <hex> --dest <path> [--repo-id <repo-id>] [--nexus-user <user>] [--store-credentials] [--nexus-password <pass>]`**:
  Creates a local profile for an application.
  - `--repo-id`: Link the profile to a specific repository.
  - `--nexus-user`: Set the username for the repository.
  - `--store-credentials`: Securely store Nexus credentials in the OS keyring for the given `repo-id`.
  - `--nexus-password`: Provide the password for storage (if omitted and not in non-interactive mode, it will be prompted with masking).
- **`get <profile> [--use-keyring] [--non-interactive] [--verbose] [--force] [--version <ver>]`**:
  Installs or updates the application.
  - **Authentication Hierarchy**:
    1. ENV variables (`ITRUST_NEXUS_USERNAME`, `ITRUST_NEXUS_PASSWORD`).
    2. OS Keyring (if `--use-keyring` is set and profile has `repo-id`).
    3. Interactive prompt (if not `--non-interactive`, supports masked password entry).
  - In non-interactive mode, if credentials are missing, it will fail with a clear message.
  - Verifies the repository public key fingerprint and the manifest signature. Performs an atomic update with a backup of the previous version.
  - Empty `--version` or `--version latest` rejects channel manifests older than the last accepted signed `generatedAt`, including when `--force` is used. A different payload with the same timestamp is also rejected.
  - An explicit `--version <ver>` allows an intentional downgrade without lowering the saved manifest checkpoint. A publisher can roll back a channel by signing a new manifest with a later `generatedAt` and the desired older application version.
- **`status <profile> [--use-keyring] [--non-interactive]`**:
  Shows installation status and checks for updates. Performs secure manifest verification using the same authentication hierarchy as `get`. If credentials are missing in non-interactive mode, latest version will be shown as `unverified`.
- **`push --artifact-path <path> [--repo-id <id>] [--app-id <id>] [--version <ver>] [--run-hooks] [--force]`**:
  Publishes a new release. Requires `itrust-updater.project.env` in the current directory or configuration via environment variables or CLI flags.
  CLI flags have the highest priority. Supports pre-push hooks (e.g., for binary signing).
  Artifacts within a version are immutable by default (protection per OS/Architecture). Use `--force` to overwrite an existing artifact for the same version and platform. Adding artifacts for new platforms to an existing version is allowed.
  Before merging an existing release, `push` verifies its signature using the public key derived from the locally configured signing seed, and checks the application, version, and repository ID (when configured). Missing/invalid signatures or mismatched metadata abort publication before any upload, even with `--force`. A key downloaded from the repository is never trusted for this check. Existing releases signed with a different key require a separately trusted migration; `--force` does not bypass signature verification.

### Utilities

- **`manifest verify --file <json> --repo-pubkey <path> [--repo-pubkey-sha256 <hex>]`**: Manually verify a manifest.
- **`manifest sign --payload <json> --out <json> --key-id <id> [--use-keyring]`**: Manually sign a payload.
- **`version`**: Displays application name, copyrights, and version.

## Multi-Repo Support

`itrust-updater` supports multiple repositories via `ITRUST_REPO_ID`. When a command is run with a specific `repo-id`, it will look for configuration in `<configDir>/repos/<repo-id>.env` and secrets in the OS keyring (service: `itrust-updater`).

Example usage:
```bash
# Initialize repo on publisher machine
itrust-updater repo init --repo-id main-repo --base-url https://nexus.example.com/repository/updates --nexus-user admin --use-keyring

# Initialize app profile linked to the repo
itrust-updater init my-app --repo-id main-repo --app-id my-app --dest ./bin/my-app
```

## Configuration

Supports `.env` style configuration files and environment variables.

Repository IDs and profile names must begin with an ASCII letter or digit; subsequent characters may be ASCII letters, digits, `.`, `_`, or `-`. Trailing dots and Windows device names such as `CON` or `LPT1` are rejected on every platform. Separators, absolute paths, spaces, and control characters are not accepted, and invalid identifiers are rejected rather than normalized. Existing configurations using other names need to be renamed consistently before use.

Repository imports validate the ID even when configuration-file writing is disabled. Repository configuration writes are atomic and confined to the `repos` directory; that directory must not be a symlink. Import errors in configuration or secret storage are reported as failures, not success with warnings. A partial import may have already saved its configuration or some secrets; its source bundle remains available for retry.

For Go integrations, `repo.GetRepoConfigPath` now returns `(string, error)` so invalid repository IDs can be reported before file access.

**Priority**: CLI args > ENV (`ITRUST_*`) > profile.env > repo.env > project.env > defaults.

### Locations
- **Linux**:
  - Config: `~/.config/itrust-updater/`
  - State: `~/.local/state/itrust-updater/`
- **Windows**:
  - Config: `%APPDATA%\itrust-updater\`
  - State: `%LOCALAPPDATA%\itrust-updater\`

### Common Environment Variables
- `ITRUST_REPO_ID`: Repository identifier.
- `ITRUST_BASE_URL`: Repository base URL.
- `ITRUST_NEXUS_USERNAME` / `ITRUST_NEXUS_PASSWORD`: Nexus credentials.
- `ITRUST_REPO_SIGNING_ED25519_SEED_B64`: Seed for signing manifests (32 bytes base64).
- `ITRUST_REPO_PUBKEY_SHA256`: Expected SHA256 fingerprint of the repository public key.
- `ITRUST_TRANSFER_IDLE_TIMEOUT`: Maximum time without HTTP transfer progress; a positive Go duration such as `30s` or `5m` (default: `2m`). Applies to `get`, `status`, `push`, and `repo init`. The latter reads this setting from the environment; other commands also accept it in their configuration files.

## Security Features

- **Mandatory Signing**: All manifests must be signed using Ed25519.
- **Key Pinning**: Fingerprint of the repository public key is verified before any update.
- **Manifest Identity**: `get` and `status` require the configured application ID and, when set, repository ID to match the signed payload. Channel requests (including `--version latest`) also require the configured channel. An explicit `--version <ver>` requires exactly that version; its original release channel may differ because release manifests are shared across channels. Publisher merges follow the same release identity checks. Unsupported schema versions and signature algorithms are rejected by these operations.
- **Manifest Replay Protection**: `get` persists the newest verified manifest timestamp and payload hash separately from installed-version state, scoped to the profile, signing-key fingerprint, application, and channel. `status` checks these saved checkpoints without advancing them.
- **Atomic Replace**: Artifacts are downloaded to a temporary file and renamed atomically.
- **Bounded Downloads**: Remote manifests are limited to 1 MiB of JSON, including whitespace, and Ed25519 public keys must contain exactly 32 raw bytes. Artifacts must match both the signed `size` in bytes and SHA256. Downloads write at most the signed size to disk and read at most one extra byte to detect an oversized response. Failed downloads, backups, and replacements remove the temporary download file.
- **Transfer Idle Timeout**: Stalled HTTP requests, uploads, and response bodies are cancelled. Transfers with regular progress may take longer than the idle timeout; existing connection and response-header timeouts still apply. Upload retries reopen the source file. A failed response body read aborts the current download without retrying a partially consumed stream.
- **Masked Input**: Passwords are read from the terminal without echoing.
- **JCS (RFC 8785)**: JSON Canonicalization Scheme is used for signing consistency.
- **Keyring**: Securely stores secrets (passwords, signing seeds) using the OS keyring (opt-in via `--use-keyring` or `--store-credentials`).

Existing manifests need an accurate artifact `size`; a missing size is treated as zero and cannot describe a nonempty artifact. `push` already fills this field and now checks the merged manifest's size before uploading anything. There is no new fixed artifact size cap in `get`: the limit comes from the verified manifest. For Go library consumers, the existing `install.InstallArtifact` signature is retained with an 8 GiB safety cap; use `install.InstallArtifactWithSize` to supply the expected size explicitly, including for larger files.

### Replay protection and existing installations

The ordering field is `payload.generatedAt`, which is covered by the Ed25519 signature; the unsigned `signature.createdAt` is not used. Publishers must give every changed channel manifest a strictly increasing timestamp. Keep publisher clocks synchronized. Application version strings need not follow semantic versioning.

Checkpoints live in `<stateDir>/manifest-history/` and are saved before downloading an artifact, including when the current installation is already up to date. An explicit historical installation preserves newer checkpoints. Invalid history or a failure to persist it stops installation. Keep this directory when migrating state: removing it, selecting a new state directory, or changing the pinned key starts a new trust history.

On upgrading an existing installation without history, `get` first verifies the manifest for the locally recorded installed version, including its application, channel, and artifact hash. That signed manifest establishes the baseline before the requested version or channel is accepted. The installed release manifest must remain available and match the saved state; otherwise the command fails instead of silently trusting the current channel. Changing the pinned key can therefore require making the installed release verifiable with the new key before migration.

A first installation has no earlier trusted checkpoint and cannot detect replay of metadata it has never seen. This mechanism also does not detect a server indefinitely withholding newer releases. It protects against replay relative to saved local history, not against compromise of the signing key or local state.

Updates of the same profile and state directory are serialized by a lock in `<stateDir>/locks/`. Normal completion releases the lock. After a crash or forced termination, the error identifies the lock directory; remove it only after confirming that no updater for that profile is still running. Protect the state directory from untrusted local modification.

## CI/CD Integration

`itrust-updater push` is designed for CI/CD environments. It can run without local config files if all required data is provided via environment variables or flags.

### GitHub Actions

```bash
# Example GitHub Actions step
env:
  ITRUST_BASE_URL: ${{ secrets.REPO_URL }}
  ITRUST_NEXUS_USERNAME: ${{ secrets.NEXUS_USER }}
  ITRUST_NEXUS_PASSWORD: ${{ secrets.NEXUS_PASS }}
  ITRUST_REPO_SIGNING_ED25519_SEED_B64: ${{ secrets.SIGNING_SEED }}
run: itrust-updater push --non-interactive --app-id my-app --version 1.0.0 --artifact-path ./build/app
```

### Bitbucket Pipelines

https://bitbucket.org/itrust-dev/itrust-updater-pipe

````yaml
pipelines:
  default:
    - step: 
        name: Build app
        script:
          - (cd cmd/my-app && make build)
        artifacts:
          - cmd/my-app
    - step:
        name: Deploy artefact to Nexus
        script:
          - CURRENT_VERSION=$(git tag -l "v*" | sort -V | tail -n 1)
          - pipe: docker://lapierre/itrust-updater-pipe:1.2.2
            variables:
              ITRUST_BASE_URL: "${ITRUST_BASE_URL}"
              ITRUST_VERSION: "${CURRENT_VERSION}"
              ITRUST_APP_ID: "${ITRUST_APP_ID}"
              ITRUST_ARTIFACT_PATH: "cmd/my-app"
              ITRUST_REPO_SIGNING_ED25519_SEED_B64: "${ITRUST_REPO_SIGNING_ED25519_SEED_B64}"
              ITRUST_NEXUS_USERNAME: ${ITRUST_NEXUS_USERNAME}
              ITRUST_NEXUS_PASSWORD: "${ITRUST_NEXUS_PASSWORD}"
````
