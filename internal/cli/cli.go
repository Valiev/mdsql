package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type CommandMode string

const (
	ModeHelp       CommandMode = "help"
	ModeQuery      CommandMode = "query"
	ModeSchema     CommandMode = "schema"
	ModeVersion    CommandMode = "version"
	ModeClearCache CommandMode = "clear-cache"
	ModeCacheInfo  CommandMode = "cache-info"
)

type Config struct {
	Mode               CommandMode
	Query              string
	Files              []string
	Format             string
	Json               bool
	MaxLines           int
	MaxRows            int
	MaxRowsSet         bool
	Null               *string
	Root               string
	Include            []string
	Exclude            []string
	Strict             bool
	Explain            bool
	Quiet              bool
	FailEmpty          bool
	Today              time.Time
	WithFile           bool
	NoFile             bool
	NoHeader           bool
	Cache              bool
	StdinName          string
	MaxFileSize        int64
	TableLimit         int
	SchemaTablesOnly   bool
	SchemaExamplesOnly bool
}

var errNeedQuery = errors.New("query is required")

func ParseArgs(argv []string) (Config, error) {
	cfg := Config{
		Format:             "text",
		MaxLines:           0,
		MaxRows:            500,
		Root:               "",
		Today:              time.Now(),
		WithFile:           true,
		Cache:              os.Getenv("MDSQL_CACHE") == "1",
		MaxFileSize:        16 << 20,
		SchemaTablesOnly:   false,
		SchemaExamplesOnly: false,
	}
	if len(argv) == 0 {
		cfg.Mode = ModeHelp
		return cfg, nil
	}

	args := make([]string, 0, len(argv))
	todayReference := cfg.Today
	clearCache := false
	cacheInfo := false
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") || len(args) == 1 && strings.EqualFold(args[0], "query") && isQueryComment(a) {
			args = append(args, a)
			continue
		}

		switch {
		case a == "-h", a == "--help":
			cfg.Mode = ModeHelp
			return cfg, nil
		case a == "-V", a == "--version":
			cfg.Mode = ModeVersion
			return cfg, nil
		case a == "--json":
			cfg.Json = true
			cfg.Format = "json"
		case a == "--csv":
			cfg.Json = false
			cfg.Format = "csv"
		case a == "--tsv":
			cfg.Json = false
			cfg.Format = "tsv"
		case a == "--null":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("--null requires value")
			}
			i++
			value := argv[i]
			cfg.Null = &value
		case strings.HasPrefix(a, "--null="):
			value := strings.TrimPrefix(a, "--null=")
			cfg.Null = &value
		case a == "--strict":
			cfg.Strict = true
		case a == "--explain":
			cfg.Explain = true
		case a == "-q", a == "--quiet":
			cfg.Quiet = true
		case a == "--fail-empty":
			cfg.FailEmpty = true
		case a == "--no-file":
			cfg.NoFile = true
		case a == "--no-header":
			cfg.NoHeader = true
		case a == "--cache":
			cfg.Cache = true
		case a == "--no-cache":
			cfg.Cache = false
		case a == "--clear-cache":
			clearCache = true
		case a == "--cache-info":
			cacheInfo = true
		case a == "--with-file":
			cfg.NoFile = false
			cfg.WithFile = true
		case a == "--tables":
			cfg.SchemaTablesOnly = true
		case a == "--examples":
			cfg.SchemaExamplesOnly = true
		case strings.HasPrefix(a, "--format="):
			cfg.Format = strings.TrimPrefix(a, "--format=")
		case a == "--format":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("--format requires value")
			}
			i++
			cfg.Format = argv[i]
		case a == "-f":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("-f requires value")
			}
			i++
			cfg.Format = argv[i]
		case strings.HasPrefix(a, "-f") && len(a) > 2:
			cfg.Format = strings.TrimPrefix(a, "-f")
		case a == "-n":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("-n requires value")
			}
			i++
			n, err := strconv.Atoi(argv[i])
			if err != nil || n < 0 {
				return Config{}, fmt.Errorf("-n requires a non-negative integer")
			}
			cfg.MaxLines = n
		case strings.HasPrefix(a, "-n") && len(a) > 2 && ((a[2] >= '0' && a[2] <= '9') || a[2] == '-'):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "-n"))
			if err != nil || n < 0 {
				return Config{}, fmt.Errorf("-n requires a non-negative integer")
			}
			cfg.MaxLines = n
		case strings.HasPrefix(a, "--root="):
			cfg.Root = strings.TrimPrefix(a, "--root=")
		case a == "--root":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("--root requires value")
			}
			i++
			cfg.Root = argv[i]
		case strings.HasPrefix(a, "--include="):
			cfg.Include = append(cfg.Include, strings.TrimPrefix(a, "--include="))
		case a == "--include":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("--include requires value")
			}
			i++
			cfg.Include = append(cfg.Include, argv[i])
		case strings.HasPrefix(a, "--exclude="):
			cfg.Exclude = append(cfg.Exclude, strings.TrimPrefix(a, "--exclude="))
		case a == "--exclude":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("--exclude requires value")
			}
			i++
			cfg.Exclude = append(cfg.Exclude, argv[i])
		case strings.HasPrefix(a, "--today="):
			_, t, err := dateFlag("--today", strings.TrimPrefix(a, "--today="), todayReference)
			if err != nil {
				return Config{}, err
			}
			cfg.Today = t
		case a == "--today":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("--today requires value")
			}
			i++
			_, t, err := dateFlag("--today", argv[i], todayReference)
			if err != nil {
				return Config{}, err
			}
			cfg.Today = t
		case strings.HasPrefix(a, "--stdin-name="):
			cfg.StdinName = strings.TrimPrefix(a, "--stdin-name=")
		case a == "--stdin-name":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("--stdin-name requires value")
			}
			i++
			cfg.StdinName = argv[i]
		case a == "--max-lines", a == "--max-rows", a == "--table-limit":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("%s requires value", a)
			}
			i++
			n, err := strconv.Atoi(argv[i])
			if err != nil || n < 0 {
				return Config{}, fmt.Errorf("%s requires a non-negative integer", a)
			}
			if a == "--max-lines" {
				cfg.MaxLines = n
			} else if a == "--max-rows" {
				cfg.MaxRows = n
				cfg.MaxRowsSet = true
			} else {
				cfg.TableLimit = n
			}
		case strings.HasPrefix(a, "--max-rows="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--max-rows="))
			if err != nil || n < 0 {
				return Config{}, fmt.Errorf("--max-rows requires a non-negative integer")
			}
			cfg.MaxRows = n
			cfg.MaxRowsSet = true
		case a == "--max-file-size":
			if i+1 >= len(argv) {
				return Config{}, fmt.Errorf("--max-file-size requires value")
			}
			i++
			v, err := strconv.ParseInt(argv[i], 10, 64)
			if err != nil {
				return Config{}, err
			}
			cfg.MaxFileSize = v
		case strings.HasPrefix(a, "--max-file-size="):
			v, err := strconv.ParseInt(strings.TrimPrefix(a, "--max-file-size="), 10, 64)
			if err != nil {
				return Config{}, err
			}
			cfg.MaxFileSize = v
		default:
			args = append(args, a)
			if aliasOptionNeedsValue(a) && i+1 < len(argv) {
				i++
				args = append(args, argv[i])
			}
		}
	}

	if clearCache || cacheInfo {
		flag := "--clear-cache"
		if cacheInfo {
			flag = "--cache-info"
		}
		if clearCache && cacheInfo {
			return Config{}, fmt.Errorf("--clear-cache and --cache-info cannot be combined")
		}
		if err := validateFlags("", args); err != nil {
			return Config{}, err
		}
		if len(args) != 0 {
			return Config{}, fmt.Errorf("%s cannot be combined with a query or input files", flag)
		}
		cfg.Mode = ModeClearCache
		if cacheInfo {
			cfg.Mode = ModeCacheInfo
		}
		return cfg, nil
	}
	if cfg.Format == "" {
		cfg.Format = "text"
	}
	switch cfg.Format {
	case "text", "json", "jsonl", "csv", "tsv", "md", "plain", "outline", "grouped":
	default:
		return Config{}, fmt.Errorf("unknown format %q", cfg.Format)
	}
	if cfg.NoHeader && cfg.Format != "csv" && cfg.Format != "tsv" {
		return Config{}, fmt.Errorf("--no-header requires csv or tsv format")
	}
	if cfg.Null != nil && cfg.Format != "csv" && cfg.Format != "tsv" && cfg.Format != "grouped" && cfg.Format != "text" {
		return Config{}, fmt.Errorf("--null requires text, grouped, csv, or tsv format")
	}
	cfg.Json = cfg.Format == "json"
	if len(args) == 0 {
		if cfg.SchemaTablesOnly {
			return cfg, errors.New("--tables requires the schema command; use `mdsql schema --tables`")
		}
		cfg.Mode = ModeHelp
		return cfg, nil
	}

	if strings.HasPrefix(args[0], "-") {
		return cfg, fmt.Errorf("unknown flag %q", args[0])
	}
	command := strings.ToLower(args[0])
	if cfg.SchemaTablesOnly && command != "schema" {
		return cfg, errors.New("--tables requires the schema command; use `mdsql schema --tables`")
	}
	if command == "help" || command == "schema" || command == "version" || command == "query" || isSelect(args[0]) {
		flagArgs := args[1:]
		if command == "query" && len(flagArgs) > 0 && isQueryComment(flagArgs[0]) {
			flagArgs = flagArgs[1:]
		}
		if err := validateFlags("", flagArgs); err != nil {
			return cfg, err
		}
	}
	switch command {
	case "help":
		cfg.Mode = ModeHelp
		return cfg, nil
	case "schema":
		cfg.Mode = ModeSchema
		cfg.Files = args[1:]
		return cfg, nil
	case "version":
		cfg.Mode = ModeVersion
		cfg.Files = args[1:]
		return cfg, nil
	case "query":
		cfg.Mode = ModeQuery
		if len(args) < 2 {
			return cfg, errors.New("query string missing")
		}
		cfg.Query = args[1]
		cfg.Files = append(cfg.Files, args[2:]...)
		return cfg, nil
	default:
		if isSelect(args[0]) {
			cfg.Mode = ModeQuery
			cfg.Query = args[0]
			cfg.Files = append(cfg.Files, args[1:]...)
			return cfg, nil
		}
		cfg.Mode = ModeQuery
		sql, files, err := aliasToQuery(strings.ToLower(args[0]), args[1:])
		if err != nil {
			return cfg, err
		}
		cfg.Query = sql
		cfg.Files = files
		return cfg, nil
	}
}

func Help() string {
	return `mdsql - SQL-запросы к Markdown и Obsidian-файлам

Использование:
  mdsql "SELECT ... FROM table" [files...]
  mdsql query "SELECT ... FROM table" [files...]
  mdsql <alias> [options] [files...]
  mdsql schema [--tables|--examples] [--json]
  mdsql version
  mdsql "INSERT INTO tasks (text) VALUES ('New task')" file.md
  mdsql "UPDATE tasks SET state = 'done' WHERE line = 3" file.md

INSERT/UPDATE (one existing regular file; writes immediately):
  --dry-run             Preview the unified diff without writing
  --expect-rows N       Require N matched entities (INSERT: 1)
  --expect-sha256 HASH  Require the original SHA-256
  --root DIR            Resolve the positional file relative to DIR
  --json, --format=json/text  Result as JSON or text

Алиасы:
  headings  tasks  code  tables  links  tags  frontmatter  files
  sections  blocks  at  lines  grep  section  toc

Основные опции:
  -h, --help             Показать эту справку
  -V, --version          Показать версию
  --csv                  Вывести результат в CSV
  --tsv                  Вывести результат в TSV
  -f, --format FORMAT    text, json, jsonl, csv, tsv, md, plain, outline, grouped
                        grouped требует file в SELECT; имя файла один раз на группу
  --no-header           Убрать заголовок колонок CSV/TSV
  --null STR            Представление NULL в text/grouped/CSV/TSV
  --cache               Включить приватный кэш разбора (также MDSQL_CACHE=1)
  --no-cache            Не читать и не записывать кэш
  --clear-cache         Очистить записи кэша mdsql и завершить работу
  --cache-info          Количество записей и размер кэша в байтах
	--quiet, -q           Не печатать предупреждения
	--fail-empty          Завершиться с кодом 3 при пустом результате
	--root DIR             Корень для файлов и относительных путей
	--include GLOB         Включить файлы по шаблону
	--exclude GLOB         Исключить файлы по шаблону
                        Оба: путь от --root; * — один уровень, ** — рекурсивно
	--today YYYY-MM-DD     Переопределить значение today
	--stdin-name NAME      Имя источника для стандартного ввода
	--max-file-size BYTES  Максимальный размер одного входного файла
	-n, --max-lines N      Сократить многострочные значения при выводе
	--max-rows N           Ограничить вывод строк (по умолчанию 500; 0 — без ограничения)
	--table-limit N        Ограничить строки после выполнения SQL (0 — без ограничения)
	--no-file              Совместимый флаг; состав SELECT определяет наличие file
                        Явный SQL LIMIT снимает только дефолт; --strict + усечение: exit 4
  --strict               Ошибка при проблеме входа или диагностике запроса
  --explain              Показать SQL после раскрытия алиаса

Примеры:
  mdsql headings notes/
  mdsql tasks --open --tag work vault/
  mdsql "SELECT title, line FROM headings WHERE level <= 2" README.md
  cat note.md | mdsql "SELECT text FROM tasks WHERE state = 'todo'"
`
}

func isSelect(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "select") || strings.HasPrefix(strings.ToUpper(strings.TrimSpace(v)), "SELECT ")
}

func joinTokens(in []string) string {
	return strings.Join(in, " ")
}

func (c *Config) ResolveStdinName() string {
	if c.StdinName != "" {
		return filepath.Clean(c.StdinName)
	}
	if env := os.Getenv("MDSQL_STDIN_NAME"); env != "" {
		return filepath.Clean(env)
	}
	return "<stdin>"
}

func aliasToQuery(alias string, args []string) (string, []string, error) {
	if _, known := aliasOptions[alias]; known {
		if err := validateFlags(alias, args); err != nil {
			return "", nil, err
		}
	}
	files := []string{}
	switch alias {
	case "headings":
		query := "SELECT level, title, line FROM headings"
		filters := []string{}
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch a {
			case "--under":
				if i+1 >= len(args) {
					return "", nil, errors.New("--under requires value")
				}
				filters = append(filters, "under('"+escape(args[i+1])+"')")
				i++
			case "-l", "--level":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("%s requires value", a)
				}
				filters = append(filters, "level = "+args[i+1])
				i++
			case "--min-level", "--max-level":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("%s requires value", a)
				}
				op := ">="
				if a == "--max-level" {
					op = "<="
				}
				filters = append(filters, "level "+op+" "+args[i+1])
				i++
			default:
				if strings.HasPrefix(a, "--") {
					continue
				}
				files = append(files, a)
			}
		}
		if len(filters) > 0 {
			query += " WHERE " + strings.Join(filters, " AND ")
		}
		return query, files, nil
	case "tasks":
		query := "SELECT line, status, text, due, path FROM tasks"
		filters := []string{}
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch a {
			case "--open":
				filters = append(filters, "state = 'todo'")
			case "--done":
				filters = append(filters, "state = 'done'")
			case "--in-progress":
				filters = append(filters, "state = 'in_progress'")
			case "--state":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("--state requires value")
				}
				filters = append(filters, "state = '"+escape(args[i+1])+"'")
				i++
			case "--tag":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("--tag requires value")
				}
				filters = append(filters, "tags HAS '"+escape(args[i+1])+"'")
				i++
			case "--depth":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("--depth requires value")
				}
				filters = append(filters, "depth = "+args[i+1])
				i++
			case "--due":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("--due requires value")
				}
				value := args[i+1]
				if bounds := strings.SplitN(value, "..", 2); len(bounds) == 2 {
					low, _, err := dateFlag(a, bounds[0], time.Now())
					if err != nil {
						return "", nil, err
					}
					high, _, err := dateFlag(a, bounds[1], time.Now())
					if err != nil {
						return "", nil, err
					}
					filters = append(filters, "due BETWEEN "+low+" AND "+high)
				} else {
					expr, _, err := dateFlag(a, value, time.Now())
					if err != nil {
						return "", nil, err
					}
					filters = append(filters, "due = "+expr)
				}
				i++
			case "--overdue":
				filters = append(filters, "due < today", "state != 'done'")
			case "--scheduled-before", "--scheduled-after":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("%s requires value", a)
				}
				op := "<"
				if a == "--scheduled-after" {
					op = ">"
				}
				expr, _, err := dateFlag(a, args[i+1], time.Now())
				if err != nil {
					return "", nil, err
				}
				filters = append(filters, "scheduled "+op+" "+expr)
				i++
			case "--priority":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("--priority requires value")
				}
				filters = append(filters, "priority = "+args[i+1])
				i++
			case "--path", "--section":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("%s requires value", a)
				}
				field := strings.TrimPrefix(a, "--")
				filters = append(filters, field+" = '"+escape(args[i+1])+"'")
				i++
			case "--has-id":
				filters = append(filters, "id != ''")
			case "--raw":
				query = "SELECT raw FROM tasks"
			default:
				if strings.HasPrefix(a, "--") {
					continue
				}
				files = append(files, a)
			}
		}
		if len(filters) > 0 {
			query += " WHERE " + strings.Join(filters, " AND ")
		}
		return query, files, nil
	case "code":
		query := "SELECT lang, start, end, code FROM code"
		langs := []string{}
		filters := []string{}
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch a {
			case "--lang":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("--lang requires value")
				}
				langs = append(langs, "'"+escape(args[i+1])+"'")
				i++
			case "--section", "--under":
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("%s requires value", a)
				}
				filters = append(filters, "section = '"+escape(args[i+1])+"'")
				i++
			default:
				if strings.HasPrefix(a, "--") {
					continue
				}
				files = append(files, a)
			}
		}
		if len(langs) > 0 {
			filters = append(filters, "lang IN ("+strings.Join(langs, ",")+")")
		}
		if len(filters) > 0 {
			query += " WHERE " + strings.Join(filters, " AND ")
		}
		return query, files, nil
	case "links":
		query := "SELECT type, target, text, line FROM links"
		filters := []string{}
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--type":
				if i+1 >= len(args) {
					return "", nil, errors.New("--type requires value")
				}
				filters = append(filters, "type = '"+escape(args[i+1])+"'")
				i++
			case "--external":
				filters = append(filters, "external = true")
			case "--internal":
				filters = append(filters, "external = false")
			case "--target":
				if i+1 >= len(args) {
					return "", nil, errors.New("--target requires value")
				}
				filters = append(filters, "target LIKE '"+escape(args[i+1])+"'")
				i++
			default:
				if !strings.HasPrefix(args[i], "-") {
					files = append(files, args[i])
				}
			}
		}
		if len(filters) > 0 {
			query += " WHERE " + strings.Join(filters, " AND ")
		}
		return query, files, nil
	case "tags":
		query := "SELECT DISTINCT tag FROM tags"
		filters := []string{}
		count := false
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--count":
				count = true
			case "--source":
				if i+1 >= len(args) {
					return "", nil, errors.New("--source requires value")
				}
				filters = append(filters, "source = '"+escape(args[i+1])+"'")
				i++
			case "--explode-frontmatter":
			default:
				if !strings.HasPrefix(args[i], "-") {
					files = append(files, args[i])
				}
			}
		}
		if count {
			query = "SELECT tag, COUNT(*) FROM tags"
		}
		if len(filters) > 0 {
			query += " WHERE " + strings.Join(filters, " AND ")
		}
		if count {
			query += " GROUP BY tag ORDER BY count DESC"
		} else {
			query += " ORDER BY tag"
		}
		return query, files, nil
	case "frontmatter":
		return "SELECT file, fm FROM frontmatter", args, nil
	case "files":
		return "SELECT file, lines, headings, tasks, open_tasks FROM files", args, nil
	case "sections":
		return "SELECT level, title, start, end, path FROM sections", args, nil
	case "blocks":
		return "SELECT kind, start, end, section FROM blocks", args, nil
	case "tables":
		return "SELECT file, start, end, raw FROM tables", args, nil
	case "at":
		if len(args) < 1 {
			return "", nil, errors.New("at expects file:line")
		}
		separator := strings.LastIndex(args[0], ":")
		if separator <= 0 || separator == len(args[0])-1 {
			return "", nil, errors.New("at expects file:line")
		}
		line, err := strconv.Atoi(args[0][separator+1:])
		if err != nil || line < 1 {
			return "", nil, errors.New("at expects a positive line number in file:line")
		}
		return fmt.Sprintf("SELECT kind, start, end, raw FROM blocks WHERE at(%d)", line), []string{args[0][:separator]}, nil
	case "toc":
		return "SELECT level, title, line FROM headings ORDER BY line", args, nil
	case "lines":
		if len(args) < 2 {
			return "", nil, errors.New("lines expects file and range")
		}
		if args[1] == "" {
			return "", nil, errors.New("lines range is empty")
		}
		from, to, ok := parseLineRange(args[1])
		if !ok {
			return "", nil, errors.New("invalid range")
		}
		q := fmt.Sprintf("SELECT n, text FROM lines WHERE n BETWEEN %d AND %d", from, to)
		return q, []string{args[0]}, nil
	case "grep":
		if len(args) < 1 {
			return "", nil, errors.New("grep expects pattern")
		}
		return fmt.Sprintf("SELECT kind, start, text FROM blocks WHERE text ~ '%s'", escape(args[0])), args[1:], nil
	case "section":
		if len(args) < 1 {
			return "", nil, errors.New("section requires title")
		}
		return fmt.Sprintf("SELECT raw FROM sections WHERE title = '%s' OR slug = '%s'", escape(args[0]), escape(args[0])), args[1:], nil
	case "schema":
		return "", nil, errors.New("schema is a top-level command")
	default:
		return "", nil, fmt.Errorf("unknown alias %q", alias)
	}
}

func parseLineRange(raw string) (int, int, bool) {
	parts := strings.Split(raw, ":")
	if len(parts) == 1 {
		parts = strings.Split(raw, "-")
	}
	if len(parts) == 1 {
		n, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, false
		}
		return n, n, true
	}
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if a > b {
		return 0, 0, false
	}
	return a, b, true
}

func escape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "'", "\\'")
}
