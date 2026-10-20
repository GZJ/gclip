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

On Linux X11, `gcopy` starts a detached clipboard owner and returns as soon as
ownership is confirmed. No clipboard manager or cpq is required. The owner waits
for requests without polling and exits when another copy replaces the clipboard
or the X11 connection closes. Pasting does not terminate it, so repeated pastes
work. It retains file URIs, not file contents. Startup failures or a five-second
startup timeout are reported as errors.

The opt-in desktop integration test temporarily changes the current clipboard:

```bash
GCLIP_X11_TEST=1 go test ./cmd/gcopy -run TestX11BackgroundClipboard -v
```
