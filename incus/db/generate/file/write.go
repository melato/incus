package file

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lxc/incus/incus/db/generate/lex"
)

// Reset an auto-generated source file, writing a new empty file header.
func Reset(path string, imports []string, buildComment string, iface bool) error {
	// A new line needs to be appended after the build comment.
	if buildComment != "" {
		buildComment = fmt.Sprintf(`%s

`, buildComment)
	}

	if iface {
		err := resetInterface(path, imports, buildComment)
		if err != nil {
			return err
		}
	}

	content := fmt.Sprintf(`%spackage %s

// The code below was generated by %s - DO NOT EDIT!

import (
`, buildComment, os.Getenv("GOPACKAGE"), os.Args[0])

	for _, uri := range imports {
		content += fmt.Sprintf("\t%q\n", uri)
	}

	content += ")\n\n"

	// FIXME: we should only import what's needed.
	content += "var _ = api.ServerEnvironment{}\n"

	bytes := []byte(content)

	var err error

	if path == "-" {
		_, err = os.Stdout.Write(bytes)
	} else {
		err = os.WriteFile(path, []byte(content), 0644)
	}

	if err != nil {
		return fmt.Errorf("Reset target source file %q: %w", path, err)
	}

	return nil
}

func resetInterface(path string, imports []string, buildComment string) error {
	if strings.HasSuffix(path, "mapper.go") {
		parts := strings.Split(path, ".")
		interfacePath := strings.Join(parts[:len(parts)-2], ".") + ".interface.mapper.go"
		content := fmt.Sprintf("%spackage %s", buildComment, os.Getenv("GOPACKAGE"))
		err := os.WriteFile(interfacePath, []byte(content), 0644)
		return err
	}

	return nil
}

// Append a code snippet to a file.
func Append(entity string, path string, snippet Snippet, iface bool) error {
	if iface {
		err := appendInterface(entity, path, snippet)
		if err != nil {
			return err
		}
	}

	buffer := newBuffer()
	buffer.N()

	err := snippet.Generate(buffer)
	if err != nil {
		return fmt.Errorf("Generate code snippet: %w", err)
	}

	var file *os.File

	if path == "-" {
		file = os.Stdout
	} else {
		file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("Open target source code file %q: %w", path, err)
		}

		defer func() { _ = file.Close() }()
	}

	bytes, err := buffer.code()
	if err != nil {
		return err
	}

	_, err = file.Write(bytes)
	if err != nil {
		return fmt.Errorf("Append snippet to target source code file %q: %w", path, err)
	}

	// Return any errors on close if file is not stdout.
	if path != "-" {
		return file.Close()
	}

	return nil
}

func appendInterface(entity string, path string, snippet Snippet) error {
	if !strings.HasSuffix(path, ".mapper.go") {
		return nil
	}

	parts := strings.Split(path, ".")
	interfacePath := strings.Join(parts[:len(parts)-2], ".") + ".interface.mapper.go"

	stat, err := os.Stat(interfacePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("could not get file info for path %q: %w", interfacePath, err)
	}

	buffer := newBuffer()

	file, err := os.OpenFile(interfacePath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("Open target source code file %q: %w", interfacePath, err)
	}

	defer func() { _ = file.Close() }()

	err = snippet.GenerateSignature(buffer)
	if err != nil {
		return fmt.Errorf("Generate interface snippet: %w", err)
	}

	bytes, err := buffer.code()
	if err != nil {
		return err
	}

	declaration := fmt.Sprintf("type %sGenerated interface {", lex.Camel(entity))
	content := make([]byte, stat.Size())
	_, err = file.Read(content)
	if err != nil {
		return fmt.Errorf("Could not read interface file %q: %w", interfacePath, err)
	}

	firstWrite := !strings.Contains(string(content), declaration)
	if firstWrite {
		// If this is the first signature write to the file, append the whole thing.
		_, err = file.WriteAt(bytes, stat.Size())
	} else {
		// If an interface already exists, just append the method, omitting everything before the first '{'.
		startIndex := 0
		for i := range bytes {
			// type ObjectGenerated interface {
			if string(bytes[i]) == "{" {
				startIndex = i + 1
				break
			}
		}
		// overwrite the closing brace.
		_, err = file.WriteAt(bytes[startIndex:], stat.Size()-2)
	}

	if err != nil {
		return fmt.Errorf("Append snippet to target source code file %q: %w", interfacePath, err)
	}

	return file.Close()
}
