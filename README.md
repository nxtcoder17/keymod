# Keymod

Keymod is a keyboard remapping tool designed to run as a system service. It utilizes `go-evdev` to interact with input devices, allowing for custom key mappings.

## Features

- Keyboard remapping
- Runs as a system service

## Installation

To install Keymod, you typically need to build the executable and set up the `keymod.service` file.

1.  **Build the executable:**

    ```bash
    go build -o bin/keymod .
    ```

2.  **Install the systemd service:**

    Copy the `keymod.service` file to your systemd service directory (e.g., `/etc/systemd/system/`):

    ```bash
    sudo cp keymod.service /etc/systemd/system/
    ```

3.  **Reload systemd and enable the service:**

    ```bash
    sudo systemctl daemon-reload
    sudo systemctl enable keymod
    sudo systemctl start keymod
    ```

## Configuration

Keymod uses `go-toml` for configuration. Details on configuration options will be provided here once the configuration structure is finalized.

## Technologies Used

- Go
- `go-evdev` for input device interaction
- `go-toml` for configuration parsing
- `fastlog` for logging
