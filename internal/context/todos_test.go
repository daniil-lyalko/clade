package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindTodos_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files with TODOs
	goFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(goFile, []byte(`package main

// TODO: implement this function
func DoSomething() {
	// FIXME: handle errors properly
	return
}

// HACK: temporary workaround
var x = 1
`), 0644))

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Len(t, todos, 3)

	// Verify TODO was found
	assert.Contains(t, todos[0].Content, "TODO")
	assert.Equal(t, "main.go", todos[0].File)
	assert.Equal(t, 3, todos[0].Line)

	// Verify FIXME was found
	assert.Contains(t, todos[1].Content, "FIXME")
	assert.Equal(t, 5, todos[1].Line)

	// Verify HACK was found
	assert.Contains(t, todos[2].Content, "HACK")
	assert.Equal(t, 9, todos[2].Line)
}

func TestFindTodos_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	// Create multiple files with TODOs
	file1 := filepath.Join(srcDir, "file1.go")
	require.NoError(t, os.WriteFile(file1, []byte("// TODO: fix this\n"), 0644))

	file2 := filepath.Join(srcDir, "file2.js")
	require.NoError(t, os.WriteFile(file2, []byte("// FIXME: refactor\n"), 0644))

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Len(t, todos, 2)

	// Verify relative paths
	assert.Contains(t, todos[0].File, "file1.go")
	assert.Contains(t, todos[1].File, "file2.js")
}

func TestFindTodos_MaxResults(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file with many TODOs
	content := `
// TODO: item 1
// TODO: item 2
// TODO: item 3
// TODO: item 4
// TODO: item 5
// TODO: item 6
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(content), 0644))

	// Request only 3 results
	todos, err := FindTodos(tmpDir, 3)

	require.NoError(t, err)
	assert.Len(t, todos, 3)
}

func TestFindTodos_SkipsVendor(t *testing.T) {
	tmpDir := t.TempDir()

	// Create vendor directory with TODO
	vendorDir := filepath.Join(tmpDir, "vendor", "pkg")
	require.NoError(t, os.MkdirAll(vendorDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(vendorDir, "lib.go"), []byte("// TODO: from vendor\n"), 0644))

	// Create regular file with TODO
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("// TODO: my code\n"), 0644))

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Len(t, todos, 1)
	assert.Contains(t, todos[0].Content, "my code")
	assert.NotContains(t, todos[0].Content, "vendor")
}

func TestFindTodos_SkipsNodeModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create node_modules with TODO
	nodeDir := filepath.Join(tmpDir, "node_modules", "some-lib")
	require.NoError(t, os.MkdirAll(nodeDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nodeDir, "index.js"), []byte("// TODO: lib\n"), 0644))

	// Create src file with TODO
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "app.js"), []byte("// TODO: app\n"), 0644))

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Len(t, todos, 1)
	assert.Contains(t, todos[0].File, "app.js")
}

func TestFindTodos_OnlyScansSupportedExtensions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files with various extensions
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "code.go"), []byte("// TODO: go\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "script.js"), []byte("// TODO: js\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "README.txt"), []byte("TODO: txt\n"), 0644)) // Not scanned
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "data.json"), []byte("TODO: json\n"), 0644))   // Not scanned

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Len(t, todos, 2)
	// Should only find .go and .js, not .txt or .json
	var extensions []string
	for _, todo := range todos {
		extensions = append(extensions, filepath.Ext(todo.File))
	}
	assert.Contains(t, extensions, ".go")
	assert.Contains(t, extensions, ".js")
	assert.NotContains(t, extensions, ".txt")
	assert.NotContains(t, extensions, ".json")
}

func TestFindTodos_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()

	content := `
// todo: lowercase
// TODO: uppercase
// ToDo: mixed case
// FIXME: also finds fixme
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(content), 0644))

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Len(t, todos, 4)
}

func TestFindTodos_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Empty(t, todos)
}

func TestFindTodos_NoTodosInFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files without TODOs
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "clean.go"), []byte("package main\n\nfunc main() {}\n"), 0644))

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Empty(t, todos)
}

func TestScanFileForTodos_XXXandBUG(t *testing.T) {
	// Verify less common TODO markers are found
	tmpDir := t.TempDir()

	content := `
// XXX: needs attention
// BUG: known issue
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(content), 0644))

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	assert.Len(t, todos, 2)
	assert.Contains(t, todos[0].Content, "XXX")
	assert.Contains(t, todos[1].Content, "BUG")
}

func TestFindTodos_RelativePaths(t *testing.T) {
	// Verify file paths are relative to search directory
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src", "components")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	todoFile := filepath.Join(srcDir, "button.tsx")
	require.NoError(t, os.WriteFile(todoFile, []byte("// TODO: style this\n"), 0644))

	todos, err := FindTodos(tmpDir, 10)

	require.NoError(t, err)
	require.Len(t, todos, 1)

	// Path should be relative, not absolute
	assert.Equal(t, filepath.Join("src", "components", "button.tsx"), todos[0].File)
	assert.NotContains(t, todos[0].File, tmpDir)
}
