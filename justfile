# Install or update the waid binary
install:
    go install ./cmd/waid

alias i := install

# Install waid, then launch the TUI
[no-exit-message]
install-run: install
    waid ui

alias ir := install-run
