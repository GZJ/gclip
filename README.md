# gclip

A set of command-line tools for copying and pasting files through the clipboard on Linux X11 and Windows.

## Install

Requires Go 1.25 or later:

```bash
go install github.com/gzj/gclip/cmd/gcopy@latest
go install github.com/gzj/gclip/cmd/gpaste@latest
go install github.com/gzj/gclip/cmd/gpaste-image@latest
go install github.com/gzj/gclip/cmd/gclipboard@latest
```

Make sure `$(go env GOPATH)/bin` is in your `PATH`.

To cross-compile the Windows executables on Linux:

```bash
make windows
```

The executables are written to `bin/windows/`.

## Usage

```bash
gcopy <file-or-directory>...  # Copy files or directories
gpaste [destination]...       # Paste files (defaults to the current directory)
gpaste-image [filename]       # Save a clipboard image as PNG
gclipboard                    # Print available clipboard formats
```
