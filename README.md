# Teabag

A Terminal User Interface (TUI) application built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) to manage AppImage files. Moves AppImages to a centralized directory and creates desktop entries for easy access through your application launcher.

## Features

- **Interactive File Browser**: Browse and select AppImage files from anywhere on your system
- **Centralized Storage**: Move AppImages to a configured directory (default: `~/Applications`)
- **Desktop Integration**: Automatically creates `.desktop` files in `/usr/share/applications`
- **Customizable Metadata**: Set application name, description, icon, and categories
- **First-Time Setup**: Guided configuration for AppImage storage location
- **Persistent Configuration**: Saves preferences to `~/.config/teabag.conf`
- **Beautiful TUI**: Clean, colorful interface with keyboard navigation

## Requirements

- Go 1.25.1 or later (for building from source)
- `pkexec` or `sudo` for creating desktop entries

## Installation

### From AUR
This package is available on the AUR as [teabag-bin](https://aur.archlinux.org/packages/teabag-bin). Use an AUR helper like [yay](https://github.com/Jguer/yay) to install!
`yay -S teabag-bin`

### From Source

```bash
git clone <repository-url>
cd teabag 
go build -o teabag
```

Optionally install system-wide:
```bash
sudo mv teabag /usr/local/bin/
```

## Usage

### Interactive Mode (File Browser)

Launch without arguments to browse and select an AppImage:

```bash
./teabag
```

**Navigation:**
- `↑/↓` or `j/k` - Navigate through files and directories
- `Enter` - Select file or enter directory
- `Backspace` - Go to parent directory
- `Ctrl+C` or `Esc` - Quit

### Direct Installation

Specify an AppImage path directly:

```bash
./teabag /path/to/app.AppImage
```

### Installation Process

1. **First-time setup** (if not configured):
   - Enter the directory where AppImages should be stored (default: `~/Applications`)

2. **Application Details**:
   - **Name** (required): Display name for the application
   - **Description** (optional): Brief description shown in launchers
   - **Icon** (optional): Path to icon file
   - **Categories** (required): Desktop categories, semicolon-separated (default: `Utility;`)

3. **Installation**:
   - AppImage is moved to the configured directory and made executable
   - Desktop entry is created in `/usr/share/applications`
   - Desktop database is updated

## Configuration

Configuration is stored in `~/.config/teabag.conf`:

```
APPIMAGE_DIR="/home/user/Applications"
```

## Example

```bash
$ ./teabag ~/Downloads/MyApp.AppImage

Teabag

Installing: MyApp.AppImage

Application name: My Application
(Press Enter to continue, Ctrl+C to quit)

...

✓ Installation complete!

AppImage: /home/user/Applications/MyApp.AppImage
Desktop entry: /usr/share/applications/my-application.desktop
```
