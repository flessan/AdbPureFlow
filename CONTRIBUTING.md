# Contributing to ADBPureFlow

First off, thank you for considering contributing to ADBPureFlow! It's people like you that make ADBPureFlow such an excellent tool for Android developers.

When contributing to this repository, please first discuss the change you wish to make via issue, email, or any other method with the owners of this repository before making a change.

Please note we have a [Code of Conduct](./CODE_OF_CONDUCT.md). Please follow it in all your interactions with the project.

---

## How Can I Contribute?

### 1. Reporting Bugs
Before creating bug reports, please search the issue tracker to see if the issue has already been reported. If it is new, open a new issue and include:
- A clear, descriptive title.
- Steps to reproduce the issue.
- Your operating system and Android version.
- Any relevant logs from the ADBPureFlow logs area or CLI console.

### 2. Suggesting Enhancements
Feature requests are always welcome! Open an issue with:
- A clear description of the feature and how it works.
- Why it would be useful to other developers.
- Mockups or designs if it includes user interface changes.

### 3. Submitting Pull Requests
- Fork the repository.
- Create a new branch from `main` (e.g., `feature/awesome-new-tool` or `fix/connection-bug`).
- Write meaningful, descriptive commit messages.
- Ensure all tests pass.
- Write tests for any new logic introduced.
- Submit a pull request describing the changes and why they are valuable.

---

## Developer Setup Guide

### Prerequisites
To compile and test ADBPureFlow, you will need:
- **Go**: Version 1.20 or newer.
- **GCC compiler** (for GUI native build dependencies):
  - **Linux**: `libgl1-mesa-dev`, `libegl1-mesa-dev`, `libx11-dev`, `libxcursor-dev`, `libxrandr-dev`, `libxinerama-dev`, `libxi-dev`, `libxxf86vm-dev`.
  - **macOS**: Xcode Command Line Tools.
  - **Windows**: MSYS2 or MinGW.

### Local Compilation

#### Running the CLI
```bash
cd CLI
go run main.go
```

#### Running the GUI
```bash
cd GUI
go run main.go app.go
```

### Running Tests
To execute unit tests:
```bash
cd CLI && go test -v ./...
cd ../GUI && go test -v ./...
```

---

## Coding Standards & Code Quality
- Follow the official Go style guidelines: [Effective Go](https://golang.org/doc/effective_go.html).
- Always run `gofmt -w .` before committing to format the codebase.
- Write explanatory comments for major functions and structure declarations.
- Check and handle all returned errors explicitly. Avoid silent failures or unused results.
- Implement unit tests for any utility parser, layout algorithm, or non-I/O bound code.
