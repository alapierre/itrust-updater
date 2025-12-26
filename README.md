# itrust-updater

Secure application updater for artifacts. Supports Ed25519 manifest signing, SHA256 artifact verification, and multiple repository management.

## Key Concepts

- **Profile**: A configuration for a specific application on a machine (e.g., `go-ksef`).
- **Repository (Repo)**: A backend storage (Nexus RAW) containing artifacts and manifests. Identified by `repo-id`.
- **Channel**: Release channel (e.g., `stable`, `beta`).
- **Security**: Mandatory signature verification and public key pinning (fingerprint verification).

## CLI Commands

### Repository Management (`repo`)

Manage repository configurations and secrets.

- **`repo init --repo-id <id> --base-url <url> --nexus-user <user>`**:
  Initializes a new repository. Generates a new Ed25519 signing key, uploads the public key to the repo, and saves local configuration.
  Use `--use-keyring` to store the generated seed and Nexus credentials securely.
- **`repo config --repo-id <id>`**:
  Displays a configuration snippet for the given repository (URL, public key path, and fingerprint).
- **`repo export --repo-id <id>`**:
  Creates a bundle (env format) to migrate repository configuration to another machine. Can include the signing seed and Nexus credentials.
- **`repo import --in <file>`**:
  Imports repository configuration and secrets from an exported bundle.

### Application Management

- **`init <profile> --app-id <id> --base-url <url> --repo-pubkey-sha256 <hex> --dest <path>`**:
  Creates a local profile for an application.
- **`get <profile>`**:
  Installs or updates the application. Verifies the repository public key fingerprint and the manifest signature. Performs an atomic update with a backup of the previous version.
- **`status <profile>`**:
  Shows installation status and checks for updates. Performs secure manifest verification.
- **`push --artifact-path <path> [--repo-id <id>] [--app-id <id>] [--version <ver>]`**:
  Publishes a new release. Requires `itrust-updater.project.env` in the current directory or configuration via environment variables or CLI flags.
  The artifact path, repo ID, app ID, and version can be provided via flags. CLI flags have the highest priority.

### Utilities

- **`manifest verify --file <json> --repo-pubkey <path>`**: Manually verify a manifest.
- **`manifest sign --payload <json> --out <json> --key-id <id>`**: Manually sign a payload.

## Multi-Repo Support

`itrust-updater` supports multiple repositories via `ITRUST_REPO_ID`. When a command is run with a specific `repo-id`, it will look for configuration in `<configDir>/repos/<repo-id>.env` and secrets in the OS keyring.

Example usage:
```bash
# Initialize repo on publisher machine
itrust-updater repo init --repo-id main-repo --base-url https://nexus.example.com/repository/updates --nexus-user admin --use-keyring

# Initialize app profile linked to the repo
itrust-updater init my-app --repo-id main-repo --app-id my-app --dest ./bin/my-app
```

## Configuration

Supports `.env` style configuration files and environment variables.

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

## Security Features

- **Mandatory Signing**: All manifests must be signed using Ed25519.
- **Key Pinning**: Fingerprint of the repository public key is verified before any update.
- **Atomic Replace**: Artifacts are downloaded to a temporary file and renamed atomically.
- **JCS (RFC 8785)**: JSON Canonicalization Scheme is used for signing consistency.
- **Keyring**: Securely stores secrets (passwords, signing seeds) using the OS keyring (opt-in via `--use-keyring`).

## CI/CD Integration

`itrust-updater push` is designed for CI/CD environments. It can run without `repo-id` or local config files if all required data is provided via environment variables.

```bash
# Example GitHub Actions step
env:
  ITRUST_BASE_URL: ${{ secrets.REPO_URL }}
  ITRUST_NEXUS_USERNAME: ${{ secrets.NEXUS_USER }}
  ITRUST_NEXUS_PASSWORD: ${{ secrets.NEXUS_PASS }}
  ITRUST_REPO_SIGNING_ED25519_SEED_B64: ${{ secrets.SIGNING_SEED }}
run: itrust-updater push --non-interactive --repo-id my-repo --app-id my-app --version 1.0.0 --artifact-path ./build/app --use-keyring
```
