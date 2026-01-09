<div align="center">
<a href="https://github.com/yudhasubki/rollingthunder/"><img src="build/appicon.png" width="120"/></a>
</div>
<h1 align="center">⚡ Rolling Thunder</h1>
<div align="center">
<strong>Rolling Thunder is a modern lightweight cross-platform database desktop manager available
for Mac, Windows, and Linux.</strong>
</div>

**Status: Under Active Development**  
Rolling Thunder is currently in its **early development phase** — the foundation is being laid.  
It's **not ready for production use** yet, but stay tuned for updates!

## 🚧 What is Rolling Thunder?

**Rolling Thunder** will be a sleek and efficient desktop app to help developers and DBAs manage their databases with ease. Built for performance, simplicity, and cross-platform compatibility.

## ✨ Features

### Connection Management
- **Color-coded connections** - Visual distinction for different environments (prod/staging/dev)
- **Save connections** - Persistent storage for quick access
- **SSL support** - SSLMode, CA Certificate, Client Cert/Key
- **Right-click context menu** - Quick delete with styled confirmation modal

### Table Management
- **Create tables** - Visual table designer with column definitions
- **Drop/Truncate tables** - Right-click context menu with confirmation
- **Auto-refresh** - Sidebar updates after table operations

### Query Editor
- **SQL editor** - Write and execute queries
- **Results viewer** - View query results in table format

## Tech Stack
- **Backend**: Go + Wails
- **Frontend**: Svelte 5 + TypeScript
- **UI**: Melt UI + Tailwind CSS
- **Database**: PostgreSQL (MySQL, SQLite coming soon)

## Getting Started

```bash
# Clone the repository
git clone https://github.com/yudhasubki/rollingthunder.git
cd rollingthunder

# Install dependencies
cd frontend && npm install && cd ..

# Run in development mode
wails dev

# Build for production
wails build
```

## Roadmap

### Completed
- [x] Connection manager with colors & SSL
- [x] Table browser with context menu
- [x] Create/Drop/Truncate tables
- [x] SQL query editor with syntax highlighting (Monaco Editor)
- [x] Autocomplete for tables and columns
- [x] Inline data editing (Create/Update/Delete rows)

### In Progress
- [ ] Query history
- [ ] DDL viewer & editor (ALTER table)
- [ ] Multi Connection

### Planned
- [ ] MySQL support
- [ ] SQLite support
- [ ] Export data (CSV, JSON, SQL)
- [ ] Dark/Light theme toggle
- [ ] Keyboard shortcuts
 
## License

MIT License - see [LICENSE](LICENSE) for details.