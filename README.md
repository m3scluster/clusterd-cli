## Go Mesos CLI

Ein neues Go-basiertes CLI für Apache Mesos mit Plugin-Unterstützung.

### Features

- Kompatibel mit Python mesos-cli API
- Plugin-System für Erweiterungen
- Tabellarische Ausgabe für Agent, Framework und Tasklisten
- Autovervollständigung über `__autocomplete__`

### Build

```bash
make
```

### Konfiguration

Standardkonfigurationspfad:
- `$MESOS_CLI_CONFIG` oder `$XDG_CONFIG_HOME/mesos-cli/config.toml`

Beispielkonfiguration (`/tmp/mesos-cli-go-config.toml`):

```toml
[master]
address = "https://devtest.lab.internal:5050"
principal = "mesos"
secret = "test"
ssl_verify = false

[agent]
ssl = false
ssl_verify = false
timeout = 10
```

### Verwendung

```bash
MESOS_CLI_CONFIG=/tmp/mesos-cli-go-config.toml ./mesos-cli agent list
MESOS_CLI_CONFIG=/tmp/mesos-cli-go-config.toml ./mesos-cli task list --all
MESOS_CLI_CONFIG=/tmp/mesos-cli-go-config.toml ./mesos-cli framework list --all
MESOS_CLI_CONFIG=/tmp/mesos-cli-go-config.toml ./mesos-cli --help
```

### Plugins

Plugins liegen im Verzeichnis `$MESOS_CLI_DIR/plugins/`. Jedes Plugin benötigt:
- `plugin.toml` mit `{name, description, executable}`
- Ausführbare Datei `executable`

Plugin-Verzeichnisstruktur:
```
plugins/
├── compose/
│   ├── compose        # ausführbare Datei
│   └── plugin.toml    # name="compose", description="...", executable="compose"
├── m3s/
│   ├── m3s            # ausführbare Datei
│   └── plugin.toml    # name="m3s", description="...", executable="m3s"
```

### Ziel

Die Go-CLI emuliert die Python mesos-cli API und erweitert sie um:
- Mesos-compose Plugin für Docker Compose Integration
- Mesos-m3s Plugin für den M3S Scheduler

Die komplette API-Referenz findest du in der Python-Dokumentation von `avmesos-cli`.