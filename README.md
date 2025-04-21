# 🌐 DNS-ER

[![Go Report Card](https://goreportcard.com/badge/github.com/Shinplex/DNS-er)](https://goreportcard.com/report/github.com/Shinplex/DNS-er)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Shinplex/DNS-er)](https://github.com/Shinplex/DNS-er)
[![License](https://img.shields.io/github/license/Shinplex/DNS-er)](LICENSE)
[![Maintenance](https://img.shields.io/badge/Maintained%3F-yes-green.svg)](https://github.com/Shinplex/DNS-er/graphs/commit-activity)

🚀 A lightweight DNS server in Go that provides custom DNS record resolution.

## ✨ Features

- 🔍 Simple DNS server that responds to A, AAAA, and other record queries
- 📝 Configuration via TOML files
- 🛠️ Customizable DNS records
- 🪶 Low resource footprint
- ⚡ High performance

## 📥 Installation

```bash
git clone https://github.com/Shinplex/DNS-er.git
cd DNS-er
go build
```

## 🚀 Usage

```bash
# Run with default config
sudo ./dns-er

# Run with custom config
sudo ./dns-er -config=/path/to/config.toml
```

## ⚙️ Configuration

The server uses TOML configuration files:

- 📄 `config.toml` - Server configuration
- 📄 `records.toml` - DNS record definitions

See the `configs/` directory for examples.

## 🧪 Testing

```bash
python3 scripts/test.py
```

## 📈 Performance

DNS-ER is designed to be lightweight and efficient, handling thousands of DNS queries per second with minimal resource usage.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📜 License

[MIT](LICENSE)