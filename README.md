# shellack
portable shell implant PoC

`shellack` is a proof-of-concept for covert command and control (C2) communication utilizing DNS queries. It demonstrates how a simple, highly portable shell script client can exfiltrate data and receive commands by interacting with a custom DNS server. This project is primarily intended for security research, such as demonstrating EDR (Endpoint Detection and Response) evasion/detection techniques or exploring covert communication channels.

## Features
-   **Covert DNS Communication:** Employs DNS SRV records for command retrieval and CAA records for data exfiltration.
-   **Stealthy Client:** The client (`shellack.sh`) leverages `nslookup` in stdin mode to prevent query data and the target DNS server's address/port from appearing in command-line arguments, enhancing stealth against process monitoring.
-   **Shell-based Client:** Designed for broad compatibility across various Unix-like systems, minimizing external dependencies.
-   **Go-based Server:** A custom, robust DNS server (`server/main.go`) written in Go to efficiently process client requests.

## Operating Systems
The client script (`shellack.sh`) aims for high portability across Unix-like systems. However, users should be aware of potential platform-specific dependencies:

*   The `stat -f %m` and `stat -f %z` commands in `shellack.sh` are BSD/macOS-specific. On Linux, these would typically be `stat -c %i` and `stat -c %s` respectively. Adaptations may be required for full functionality on non-BSD/macOS Unix variants.
*   The `read -n1` command within the `decode_diag` function is a bashism. While commonly available, it is not strictly POSIX `sh`.

Confirmed compatibility (with potential `stat` adaptations):

*   🐦 AIX
*   🔥 BeOS
*   🦊 Fedora Linux
*   😈 FreeBSD
*   🧠 Gentoo Linux
*   🐂 GNU/Hurd
*   🦑 HP-UX
*   🌈 IRIX
*   🍎 NeXTSTEP
*   🔷 NetBSD
*   🐡 OpenBSD
*   🎩 RHEL (Red Hat Enterprise Linux)
*   ☀️ Solaris
*   🐧 Ubuntu Linux
*   🌐 Ultrix
*   🧮 UNICOS
*   🐙 Wolfi Linux

## Setup and Usage

### Prerequisites
*   Go (for building the server and running tests)
*   Standard Unix utilities (e.g., `nslookup`, `gzip`, `openssl`, `tr`, `sed`, `awk`, `cut`, `sort`, `xargs`, `mktemp`, `stat`, `rm`, `cksum`, `split`)

### Build the Server

```bash
cd server
go build -o ../server_binary .
cd ..
```

### Run the Server

```bash
./server_binary [port]
# Default port is 53. Use a high port (e.g., 10553) for non-root execution.
# Example:
./server_binary 10553
```

### Configure and Run the Client

1.  **Create a client configuration file.** The client expects a file named `shellack.aiff` (or `"${MAGIC_WORD}.aiff"` if the `MAGIC_WORD` environment variable is set) in a writable path like `/tmp`. This file contains the *obfuscated* IP and port of the DNS server. The client uses `tr 012. .201` to deobfuscate this string.

    To generate the correct obfuscated content for `"-port=10553 127.0.0.1"`, you can use the `make config` command:
    
    ```bash
    make config CONTENT="-port=10553 127.0.0.1" > /tmp/shellack.aiff
    ```

2.  **Run the client:**

    ```bash
    ./shellack.sh
    ```
    
    The client will start beaconing, attempting to fetch and execute commands from the DNS server, and exfiltrating results.

## Development

### Run Tests

```bash
make test
```

### Lint Code

```bash
make lint
```