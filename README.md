# getgo

A command-line tool for downloading, installing, and managing Go versions across multiple platforms.

![License](https://img.shields.io/badge/license-MIT-blue.svg)

## Overview

`getgo` simplifies the process of downloading and setting up Go environments on Linux, macOS, and Windows. It allows you to:

- Download any version of Go (including the latest)
- Extract it to a specified location (install_path)
- Set GOROOT to the versioned Go directory (install_path/go[version])
- Automatically set up the necessary environment variables (optional)
- Customize your GOPATH location
- Create a `.envrc` file for use with direnv

## Features

- Cross-platform support (Linux, macOS, Windows)
- Automatic detection of OS and architecture
- Support for downloading the latest stable version
- Home directory expansion in paths (`~` and `~/path`)
- Progress bar with download status
- Colored output for better readability
- Proper extraction for both .tar.gz (Linux/macOS) and .zip (Windows) archives
- **Optional environment variable setup** in shell configuration files
- Customizable GOPATH location
- Support for direnv via `.envrc` file generation

## Installation

### From Source

1. Clone the repository

2. Build the binary:
   ```
   go build -o getgo
   ```

3. Move the binary to a location in your PATH:
   ```
   # Linux/macOS
   sudo mv getgo /usr/local/bin/

   # Windows (run as Administrator)
   move getgo.exe C:\Windows\System32\
   ```

## Usage

```
getgo [options] [version] [install_path]
```

### Examples

- Install the latest Go version in the current directory:
  ```
  getgo
  ```

- Install the latest Go version in a specific directory:
  ```
  getgo latest ~/.go
  # or 
  getgo - ~/.go
  ```

- Install a specific Go version in the current directory:
  ```
  getgo 1.23.1
  ```

- Install a specific Go version in a specific directory:
  ```
  getgo 1.23.1 /usr/local/go
  ```

- Install a specific Go version in your home directory:
  ```
  getgo 1.23.1 ~
  ```

- Install Go with a custom GOPATH:
  ```
  getgo --path ~/custom/gopath
  # or using the shorthand
  getgo -p ~/custom/gopath
  ```

- Install Go with automatic environment setup:
  ```
  getgo -u
  # or
  getgo --unattended
  ```

- Create a .envrc file for direnv:
  ```
  getgo --envrc .
  # or specify a different path
  getgo --envrc ~/project
  ```

### Options

- `-h`, `--help`: Show usage information
- `-u`, `--unattended`: Automatically set up environment variables (default: disabled)
- `-p`, `--path PATH`: Set custom GOPATH (default is $HOME/go)
- `--envrc PATH`: Create or update a .envrc file with Go environment variables at the specified path

## Automatic Environment Setup

By default, `getgo` will not modify your environment variables. If you want to automatically set up the required environment variables, use the `-u` or `--unattended` flag:

### On Linux/macOS

- Detects your shell type (bash, zsh, fish)
- Adds the necessary export statements to your shell configuration file (`.bashrc`, `.zshrc`, etc.)
- Provides instructions on how to apply the changes to your current shell

### On Windows

- Uses PowerShell to set user-level environment variables
- Updates your PATH to include Go binary directories
- No manual configuration required

## Using with direnv

If you prefer to use [direnv](https://direnv.net/) to manage your environment variables, you can use the `--envrc` flag to create or update a `.envrc` file:

```
getgo --envrc .
```

This will:
- Create a new `.envrc` file if it doesn't exist
- Append Go environment variables to an existing `.envrc` file if it doesn't already contain Go settings
- Leave the file unchanged if it already contains Go environment variables

This behavior ensures that your existing direnv setup is preserved while adding the necessary Go environment variables.

After the `.envrc` file is created or updated, you can use `direnv allow` to enable the environment variables.

## Environment Variables

The following environment variables are set up by `getgo` when using the `-u` flag or `--envrc`:

### Linux/macOS

```
export GOROOT=/install_path/go[version]  # e.g., /home/user/.go/go1.23.1
export GOPATH=/path/to/custom/gopath     # Customizable with --path flag
export PATH=$PATH:$GOPATH/bin:$GOROOT/bin
```

### Windows

```
GOROOT=C:\install_path\go[version]       # e.g., C:\Users\user\.go\go1.23.1
GOPATH=C:\path\to\custom\gopath          # Customizable with --path flag
PATH=%PATH%;%GOPATH%\bin;%GOROOT%\bin
```

## How It Works

1. Determines the appropriate Go version to download (latest or specified)
2. Checks if the version already exists at the destination
3. Downloads the appropriate archive for your OS and architecture
4. Extracts the archive to the specified installation directory
5. Sets up the directory structure with versioned Go installations (e.g., install_path/go1.23.1)
6. Sets GOROOT to point to the versioned Go directory (install_path/go[version])
7. Optionally configures environment variables in your shell configuration files (with `-u` flag)
8. Optionally creates or updates a `.envrc` file for use with direnv (with `--envrc` flag), preserving existing content

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. 
