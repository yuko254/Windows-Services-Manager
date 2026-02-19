## Windows Service Manager

A modern Windows background service management tool that supports most of NSSM's functionality and features a beautiful visual operation interface. It is a perfect alternative to NSSM.

## Features

### 🚀 Core Functions
- **Service Management**: Register any `exe` program to run as a background service
- **Run Hidden**: Run services with the terminal window hidden
- **Startup Parameters**: Support adding startup parameters for services
- **Working Directory**: Support customizing the service working directory
- **Process Control**: Start, stop, and auto-start at boot
- **Multi-Service Support**: Manage multiple services, exiting the GUI program does not affect background services

## Technical Architecture

- **Backend**: Go 1.24
- **Frontend**: React + TypeScript
- **UI Framework**: Fluent UI React Components
- **Desktop Framework**: Wails 2.10+
- **System Tray**: Systray
- **Icons**: Fluent UI Icons

## System Requirements

- Windows 10 +
- Windows Server 2016 +
- WebView2 Runtime (usually pre-installed)

## Download and Usage

Directly download the `exe` program from [releases](https://github.com/sky22333/services/releases) to use it.

## Build Instructions

#### Environment Setup
    # Install Wails CLI
    go install github.com/wailsapp/wails/v2/cmd/wails@latest

    # Install Node.js dependencies
    cd frontend && npm install

#### Production Build
    wails build

#### Development Mode
    wails dev