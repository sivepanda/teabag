package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	configFile = ".config/appimage-installer.conf"
)

type step int

const (
	stepFileBrowser step = iota
	stepAppImageDir
	stepAppName
	stepDescription
	stepIcon
	stepCategories
	stepProcessing
	stepComplete
	stepError
)

type installCompleteMsg struct {
	err error
}

type fileEntry struct {
	name  string
	path  string
	isDir bool
}

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	titleStyle   = lipgloss.NewStyle().Bold(true)
)

type model struct {
	currentStep     step
	appImagePath    string
	appImageDir     string
	appName         string
	description     string
	iconPath        string
	categories      string
	input           string
	error           string
	message         string
	configPath      string
	firstTimeSetup  bool
	desktopFilePath string

	// File browser fields
	currentDir string
	files      []fileEntry
	cursor     int
}

func initialModel(appImagePath string) model {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, configFile)

	m := model{
		appImagePath: appImagePath,
		configPath:   configPath,
		categories:   "Utility;",
	}

	// If no app image path provided, start with file browser
	if appImagePath == "" {
		m.currentStep = stepFileBrowser
		m.currentDir, _ = os.Getwd()
		m.loadDirectory()
		return m
	}

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		m.firstTimeSetup = true
		m.currentStep = stepAppImageDir
		m.input = filepath.Join(homeDir, "Applications")
	} else {
		// Load existing config
		if dir, err := loadConfig(configPath); err == nil {
			m.appImageDir = dir
			m.currentStep = stepAppName
		} else {
			m.currentStep = stepError
			m.error = fmt.Sprintf("Failed to load config: %v", err)
		}
	}

	return m
}

func loadConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Parse APPIMAGE_DIR="path"
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "APPIMAGE_DIR=") {
			dir := strings.TrimPrefix(line, "APPIMAGE_DIR=")
			dir = strings.Trim(dir, "\"")
			return dir, nil
		}
	}

	return "", fmt.Errorf("APPIMAGE_DIR not found in config")
}

func saveConfig(path, appImageDir string) error {
	dirPath := filepath.Dir(path)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	content := fmt.Sprintf("APPIMAGE_DIR=\"%s\"\n", appImageDir)
	return os.WriteFile(path, []byte(content), 0644)
}

func (m *model) loadDirectory() {
	m.files = []fileEntry{}
	m.cursor = 0

	entries, err := os.ReadDir(m.currentDir)
	if err != nil {
		m.error = fmt.Sprintf("Failed to read directory: %v", err)
		return
	}

	// Add parent directory entry if not at root
	if m.currentDir != "/" {
		m.files = append(m.files, fileEntry{
			name:  "..",
			path:  filepath.Dir(m.currentDir),
			isDir: true,
		})
	}

	// Add directories and AppImage files
	for _, entry := range entries {
		fullPath := filepath.Join(m.currentDir, entry.Name())

		if entry.IsDir() {
			m.files = append(m.files, fileEntry{
				name:  entry.Name(),
				path:  fullPath,
				isDir: true,
			})
		} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
			m.files = append(m.files, fileEntry{
				name:  entry.Name(),
				path:  fullPath,
				isDir: false,
			})
		}
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "up", "k":
			if m.currentStep == stepFileBrowser && m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.currentStep == stepFileBrowser && m.cursor < len(m.files)-1 {
				m.cursor++
			}

		case "enter":
			newModel, cmd := m.handleEnter()
			return newModel, cmd

		case "backspace":
			if m.currentStep == stepFileBrowser {
				// Go to parent directory
				if m.currentDir != "/" {
					m.currentDir = filepath.Dir(m.currentDir)
					m.loadDirectory()
				}
			} else if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}

		default:
			if m.currentStep != stepProcessing && m.currentStep != stepComplete && m.currentStep != stepError && m.currentStep != stepFileBrowser {
				m.input += msg.String()
			}
		}

	case installCompleteMsg:
		if msg.err != nil {
			m.currentStep = stepError
			m.error = msg.err.Error()
		} else {
			m.currentStep = stepComplete
		}
		return m, nil
	}

	return m, nil
}

func (m model) handleEnter() (model, tea.Cmd) {
	switch m.currentStep {
	case stepFileBrowser:
		if len(m.files) == 0 {
			return m, nil
		}

		selected := m.files[m.cursor]
		if selected.isDir {
			// Navigate into directory
			m.currentDir = selected.path
			m.loadDirectory()
		} else {
			// AppImage file selected
			m.appImagePath = selected.path

			// Load config and proceed
			homeDir, _ := os.UserHomeDir()
			if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
				m.firstTimeSetup = true
				m.currentStep = stepAppImageDir
				m.input = filepath.Join(homeDir, "Applications")
			} else {
				if dir, err := loadConfig(m.configPath); err == nil {
					m.appImageDir = dir
					m.currentStep = stepAppName
				} else {
					m.currentStep = stepError
					m.error = fmt.Sprintf("Failed to load config: %v", err)
				}
			}
		}

	case stepAppImageDir:
		if m.input == "" {
			homeDir, _ := os.UserHomeDir()
			m.input = filepath.Join(homeDir, "Applications")
		}
		m.appImageDir = m.input

		// Create directory
		if err := os.MkdirAll(m.appImageDir, 0755); err != nil {
			m.currentStep = stepError
			m.error = fmt.Sprintf("Failed to create directory: %v", err)
			return m, nil
		}

		// Save config
		if err := saveConfig(m.configPath, m.appImageDir); err != nil {
			m.currentStep = stepError
			m.error = fmt.Sprintf("Failed to save config: %v", err)
			return m, nil
		}

		m.currentStep = stepAppName
		m.input = ""

	case stepAppName:
		if m.input == "" {
			m.error = "Application name is required"
			return m, nil
		}
		m.appName = m.input
		m.currentStep = stepDescription
		m.input = ""
		m.error = ""

	case stepDescription:
		m.description = m.input
		m.currentStep = stepIcon
		m.input = ""

	case stepIcon:
		if m.input != "" {
			if _, err := os.Stat(m.input); os.IsNotExist(err) {
				m.error = fmt.Sprintf("Warning: Icon file not found: %s", m.input)
				return m, nil
			}
		}
		m.iconPath = m.input
		m.currentStep = stepCategories
		m.input = "Utility;"
		m.error = ""

	case stepCategories:
		if m.input == "" {
			m.input = "Utility;"
		}
		// Ensure categories end with semicolon
		if !strings.HasSuffix(m.input, ";") {
			m.input += ";"
		}
		m.categories = m.input
		m.currentStep = stepProcessing
		m.input = ""

		// Perform installation
		return m, m.install()

	case stepComplete, stepError:
		return m, tea.Quit
	}

	return m, nil
}

func (m *model) install() tea.Cmd {
	// Get absolute path
	absPath, err := filepath.Abs(m.appImagePath)
	if err != nil {
		return func() tea.Msg {
			return installCompleteMsg{err: fmt.Errorf("failed to get absolute path: %w", err)}
		}
	}

	appImageFilename := filepath.Base(absPath)
	destFile := filepath.Join(m.appImageDir, appImageFilename)

	// Move AppImage to central location
	if absPath != destFile {
		if err := os.Rename(absPath, destFile); err != nil {
			return func() tea.Msg {
				return installCompleteMsg{err: fmt.Errorf("failed to move file: %w", err)}
			}
		}
	}

	// Make executable
	if err := os.Chmod(destFile, 0755); err != nil {
		return func() tea.Msg {
			return installCompleteMsg{err: fmt.Errorf("failed to make executable: %w", err)}
		}
	}

	// Build desktop entry content
	desktopEntry := fmt.Sprintf("[Desktop Entry]\nName=%s\nExec=%s\nType=Application\nCategories=%s\n",
		m.appName, destFile, m.categories)

	if m.description != "" {
		desktopEntry += fmt.Sprintf("Comment=%s\n", m.description)
	}

	if m.iconPath != "" {
		desktopEntry += fmt.Sprintf("Icon=%s\n", m.iconPath)
	}

	// Create desktop entry in temp location first
	desktopFilename := strings.ToLower(strings.ReplaceAll(m.appName, " ", "-")) + ".desktop"
	tmpDesktopFile := filepath.Join(os.TempDir(), desktopFilename)

	if err := os.WriteFile(tmpDesktopFile, []byte(desktopEntry), 0644); err != nil {
		return func() tea.Msg {
			return installCompleteMsg{err: fmt.Errorf("failed to create temp desktop entry: %w", err)}
		}
	}

	desktopDir := "/usr/share/applications"
	m.desktopFilePath = filepath.Join(desktopDir, desktopFilename)

	// Check if pkexec is available
	if _, err := exec.LookPath("pkexec"); err == nil {
		// Use pkexec (shows graphical prompt, doesn't interrupt TUI)
		return func() tea.Msg {
			// Copy desktop file
			cmd := exec.Command("pkexec", "cp", tmpDesktopFile, m.desktopFilePath)
			if err := cmd.Run(); err != nil {
				return installCompleteMsg{err: fmt.Errorf("failed to copy desktop file: %w", err)}
			}

			// Update desktop database
			if _, err := exec.LookPath("update-desktop-database"); err == nil {
				cmd := exec.Command("pkexec", "update-desktop-database", desktopDir)
				cmd.Run() // Ignore errors
			}

			m.message = fmt.Sprintf("Installation complete! %s should now appear in your application launcher", m.appName)
			return installCompleteMsg{err: nil}
		}
	}

	// Fall back to sudo with tea.ExecProcess (suspends TUI temporarily)
	return tea.Sequence(
		tea.ExecProcess(exec.Command("sudo", "cp", tmpDesktopFile, m.desktopFilePath), func(err error) tea.Msg {
			if err != nil {
				return installCompleteMsg{err: fmt.Errorf("failed to copy desktop file: %w", err)}
			}

			// Update desktop database
			if _, err := exec.LookPath("update-desktop-database"); err == nil {
				cmd := exec.Command("sudo", "update-desktop-database", desktopDir)
				cmd.Run() // Ignore errors
			}

			m.message = fmt.Sprintf("Installation complete! %s should now appear in your application launcher", m.appName)
			return installCompleteMsg{err: nil}
		}),
	)
}

func (m model) View() string {
	var s strings.Builder

	s.WriteString(titleStyle.Render("AppImage Installer") + "\n\n")

	switch m.currentStep {
	case stepFileBrowser:
		s.WriteString(fmt.Sprintf("Current directory: %s\n\n", m.currentDir))

		if len(m.files) == 0 {
			s.WriteString(infoStyle.Render("No .appimage files or directories found") + "\n")
		} else {
			for i, file := range m.files {
				cursor := " "
				if i == m.cursor {
					cursor = ">"
				}

				icon := "📄"
				if file.isDir {
					icon = "📁"
				}

				line := fmt.Sprintf("%s %s %s", cursor, icon, file.name)
				if i == m.cursor {
					line = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true).Render(line)
				}
				s.WriteString(line + "\n")
			}
		}

		s.WriteString("\n(↑/↓ or j/k: navigate, Enter: select, Backspace: parent dir, Ctrl+C: quit)")

	case stepAppImageDir:
		s.WriteString(infoStyle.Render("First-time setup: Configure AppImage storage location") + "\n\n")
		s.WriteString(fmt.Sprintf("Enter AppImage directory path: %s\n", m.input))
		s.WriteString("\n(Press Enter to use default, Ctrl+C to quit)")

	case stepAppName:
		s.WriteString(fmt.Sprintf("Installing: %s\n\n", filepath.Base(m.appImagePath)))
		s.WriteString(fmt.Sprintf("Application name: %s\n", m.input))
		if m.error != "" {
			s.WriteString("\n" + errorStyle.Render("✗ "+m.error))
		}
		s.WriteString("\n(Press Enter to continue, Ctrl+C to quit)")

	case stepDescription:
		s.WriteString(fmt.Sprintf("Installing: %s\n\n", filepath.Base(m.appImagePath)))
		s.WriteString(fmt.Sprintf("Description (optional): %s\n", m.input))
		s.WriteString("\n(Press Enter to continue, Ctrl+C to quit)")

	case stepIcon:
		s.WriteString(fmt.Sprintf("Installing: %s\n\n", filepath.Base(m.appImagePath)))
		s.WriteString(fmt.Sprintf("Icon path (optional, press Enter to skip): %s\n", m.input))
		if m.error != "" {
			s.WriteString("\n" + errorStyle.Render("✗ "+m.error))
		}
		s.WriteString("\n(Press Enter to continue, Ctrl+C to quit)")

	case stepCategories:
		s.WriteString(fmt.Sprintf("Installing: %s\n\n", filepath.Base(m.appImagePath)))
		s.WriteString(infoStyle.Render("Common categories: Utility, Development, Graphics, Network, Office, AudioVideo, Game") + "\n\n")
		s.WriteString(fmt.Sprintf("Categories (semicolon-separated): %s\n", m.input))
		s.WriteString("\n(Press Enter to install, Ctrl+C to quit)")

	case stepProcessing:
		s.WriteString(infoStyle.Render("➜ Processing...") + "\n")

	case stepComplete:
		s.WriteString(successStyle.Render("✓ Installation complete!") + "\n\n")
		s.WriteString(fmt.Sprintf("AppImage: %s\n", filepath.Join(m.appImageDir, filepath.Base(m.appImagePath))))
		s.WriteString(fmt.Sprintf("Desktop entry: %s\n\n", m.desktopFilePath))
		s.WriteString(m.message + "\n\n")
		s.WriteString("(Press any key to exit)")

	case stepError:
		s.WriteString(errorStyle.Render("✗ Error: "+m.error) + "\n\n")
		s.WriteString("(Press any key to exit)")
	}

	return s.String()
}

func main() {
	var appImagePath string

	if len(os.Args) >= 2 {
		appImagePath = os.Args[1]

		// Check if file exists
		if _, err := os.Stat(appImagePath); os.IsNotExist(err) {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ File not found: %s", appImagePath)))
			os.Exit(1)
		}
	}

	p := tea.NewProgram(initialModel(appImagePath))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
