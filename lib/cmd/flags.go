package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mithrandie/go-text"
	txjson "github.com/mithrandie/go-text/json"
)

const (
	VariableSign            = "@"
	FlagSign                = "@@"
	EnvironmentVariableSign = "@%"
	RuntimeInformationSign  = "@#"
)
const DelimitAutomatically = "SPACES"

const (
	RepositoryFlag               = "REPOSITORY"
	TimezoneFlag                 = "TIMEZONE"
	DatetimeFormatFlag           = "DATETIME_FORMAT"
	AnsiQuotesFlag               = "ANSI_QUOTES"
	WaitTimeoutFlag              = "WAIT_TIMEOUT"
	ImportFormatFlag             = "IMPORT_FORMAT"
	DelimiterFlag                = "DELIMITER"
	DelimiterPositionsFlag       = "DELIMITER_POSITIONS"
	JsonQueryFlag                = "JSON_QUERY"
	EncodingFlag                 = "ENCODING"
	NoHeaderFlag                 = "NO_HEADER"
	WithoutNullFlag              = "WITHOUT_NULL"
	StripEndingLineBreakFlag     = "STRIP_ENDING_LINE_BREAK"
	FormatFlag                   = "FORMAT"
	ExportEncodingFlag           = "WRITE_ENCODING"
	ExportDelimiterFlag          = "WRITE_DELIMITER"
	ExportDelimiterPositionsFlag = "WRITE_DELIMITER_POSITIONS"
	WithoutHeaderFlag            = "WITHOUT_HEADER"
	LineBreakFlag                = "LINE_BREAK"
	EncloseAllFlag               = "ENCLOSE_ALL"
	JsonEscapeFlag               = "JSON_ESCAPE"
	PrettyPrintFlag              = "PRETTY_PRINT"
	EastAsianEncodingFlag        = "EAST_ASIAN_ENCODING"
	CountDiacriticalSignFlag     = "COUNT_DIACRITICAL_SIGN"
	CountFormatCodeFlag          = "COUNT_FORMAT_CODE"
	ColorFlag                    = "COLOR"
	QuietFlag                    = "QUIET"
	LimitRecursion               = "LIMIT_RECURSION"
	CPUFlag                      = "CPU"
	StatsFlag                    = "STATS"
)

var FlagList = []string{
	RepositoryFlag,
	TimezoneFlag,
	DatetimeFormatFlag,
	AnsiQuotesFlag,
	WaitTimeoutFlag,
	ImportFormatFlag,
	DelimiterFlag,
	DelimiterPositionsFlag,
	JsonQueryFlag,
	EncodingFlag,
	NoHeaderFlag,
	WithoutNullFlag,
	StripEndingLineBreakFlag,
	FormatFlag,
	ExportEncodingFlag,
	ExportDelimiterFlag,
	ExportDelimiterPositionsFlag,
	WithoutHeaderFlag,
	LineBreakFlag,
	EncloseAllFlag,
	JsonEscapeFlag,
	PrettyPrintFlag,
	EastAsianEncodingFlag,
	CountDiacriticalSignFlag,
	CountFormatCodeFlag,
	ColorFlag,
	QuietFlag,
	LimitRecursion,
	CPUFlag,
	StatsFlag,
}

type Format int

const (
	AutoSelect Format = -1 + iota
	CSV
	TSV
	FIXED
	JSON
	LTSV
	GFM
	ORG
	TEXT
)

var FormatLiteral = map[Format]string{
	CSV:   "CSV",
	TSV:   "TSV",
	FIXED: "FIXED",
	JSON:  "JSON",
	LTSV:  "LTSV",
	GFM:   "GFM",
	ORG:   "ORG",
	TEXT:  "TEXT",
}

func (f Format) String() string {
	return FormatLiteral[f]
}

var ImportFormats = []Format{
	CSV,
	TSV,
	FIXED,
	JSON,
	LTSV,
}

var JsonEscapeTypeLiteral = map[txjson.EscapeType]string{
	txjson.Backslash:        "BACKSLASH",
	txjson.HexDigits:        "HEX",
	txjson.AllWithHexDigits: "HEXALL",
}

func JsonEscapeTypeToString(escapeType txjson.EscapeType) string {
	return JsonEscapeTypeLiteral[escapeType]
}

const (
	CsvExt      = ".csv"
	TsvExt      = ".tsv"
	JsonExt     = ".json"
	LtsvExt     = ".ltsv"
	GfmExt      = ".md"
	OrgExt      = ".org"
	SqlExt      = ".sql"
	CsvqProcExt = ".cql"
	TextExt     = ".txt"
)

type ImportOptions struct {
	Format             Format
	Delimiter          rune
	DelimiterPositions []int
	SingleLine         bool
	JsonQuery          string
	Encoding           text.Encoding
	NoHeader           bool
	WithoutNull        bool
}

func (ops ImportOptions) Copy() ImportOptions {
	var dp []int
	if ops.DelimiterPositions != nil {
		dp = make([]int, len(ops.DelimiterPositions))
		copy(dp, ops.DelimiterPositions)
	}

	ret := ops
	ret.DelimiterPositions = dp
	return ret
}

func NewImportOptions() ImportOptions {
	return ImportOptions{
		Format:             CSV,
		Delimiter:          ',',
		DelimiterPositions: nil,
		SingleLine:         false,
		JsonQuery:          "",
		Encoding:           text.AUTO,
		NoHeader:           false,
		WithoutNull:        false,
	}
}

type ExportOptions struct {
	StripEndingLineBreak bool
	Format               Format
	Encoding             text.Encoding
	Delimiter            rune
	DelimiterPositions   []int
	SingleLine           bool
	WithoutHeader        bool
	LineBreak            text.LineBreak
	EncloseAll           bool
	JsonEscape           txjson.EscapeType
	PrettyPrint          bool

	// For Calculation of String Width
	EastAsianEncoding    bool
	CountDiacriticalSign bool
	CountFormatCode      bool

	Color bool
}

func (ops ExportOptions) Copy() ExportOptions {
	var dp []int
	if ops.DelimiterPositions != nil {
		dp = make([]int, len(ops.DelimiterPositions))
		copy(dp, ops.DelimiterPositions)
	}

	ret := ops
	ret.DelimiterPositions = dp
	return ret
}

func NewExportOptions() ExportOptions {
	return ExportOptions{
		StripEndingLineBreak: false,
		Format:               TEXT,
		Encoding:             text.UTF8,
		Delimiter:            ',',
		DelimiterPositions:   nil,
		SingleLine:           false,
		WithoutHeader:        false,
		LineBreak:            text.LF,
		EncloseAll:           false,
		JsonEscape:           txjson.Backslash,
		PrettyPrint:          false,
		EastAsianEncoding:    false,
		CountDiacriticalSign: false,
		CountFormatCode:      false,
		Color:                false,
	}
}

type Flags struct {
	// Common Settings
	Repository     string
	Location       string
	DatetimeFormat []string
	AnsiQuotes     bool

	WaitTimeout float64

	// For Import
	ImportOptions ImportOptions

	// For Export
	ExportOptions ExportOptions

	// System Use
	Quiet          bool
	LimitRecursion int64
	CPU            int
	Stats          bool
}

func GetDefaultNumberOfCPU() int {
	n := runtime.NumCPU() / 2
	if n < 1 {
		n = 1
	}
	return n
}

func NewFlags(env *Environment) *Flags {
	var datetimeFormat []string
	if env != nil {
		datetimeFormat = make([]string, 0, len(env.DatetimeFormat))
		for _, v := range env.DatetimeFormat {
			datetimeFormat = AppendStrIfNotExist(datetimeFormat, v)
		}
	} else {
		datetimeFormat = make([]string, 0, 4)
	}

	return &Flags{
		Repository:     "",
		Location:       "Local",
		DatetimeFormat: datetimeFormat,
		AnsiQuotes:     false,
		WaitTimeout:    10,
		ImportOptions:  NewImportOptions(),
		ExportOptions:  NewExportOptions(),
		Quiet:          false,
		LimitRecursion: 1000,
		CPU:            GetDefaultNumberOfCPU(),
		Stats:          false,
	}
}

func (f *Flags) SetRepository(s string) error {
	if len(s) < 1 {
		f.Repository = ""
		return nil
	}

	path, err := filepath.Abs(s)
	if err != nil {
		path = s
	}

	stat, err := os.Stat(path)
	if err != nil {
		return errors.New("repository does not exist")
	}
	if !stat.IsDir() {
		return errors.New("repository must be a directory path")
	}

	f.Repository = path
	return nil
}

func (f *Flags) SetLocation(s string) error {
	if len(s) < 1 || strings.EqualFold(s, "Local") {
		s = "Local"
	} else if strings.EqualFold(s, "UTC") {
		s = "UTC"
	}

	loc, err := time.LoadLocation(s)
	if err != nil {
		return errors.New(fmt.Sprintf("timezone %q does not exist", s))
	}

	f.Location = s
	location = loc
	return nil
}

func (f *Flags) SetDatetimeFormat(s string) {
	if len(s) < 1 {
		return
	}

	var formats []string
	if err := json.Unmarshal([]byte(s), &formats); err == nil {
		for _, v := range formats {
			f.DatetimeFormat = AppendStrIfNotExist(f.DatetimeFormat, v)
		}
	} else {
		f.DatetimeFormat = append(f.DatetimeFormat, s)
	}
}

func (f *Flags) SetAnsiQuotes(b bool) {
	f.AnsiQuotes = b
}

func (f *Flags) SetWaitTimeout(t float64) {
	if t < 0 {
		t = 0
	}

	f.WaitTimeout = t
	return
}

func (f *Flags) SetImportFormat(s string) error {
	fm, _, err := ParseFormat(s, f.ExportOptions.JsonEscape)
	if err != nil {
		return errors.New("import format must be one of CSV|TSV|FIXED|JSON|LTSV")
	}

	switch fm {
	case CSV, TSV, FIXED, JSON, LTSV:
		f.ImportOptions.Format = fm
		return nil
	}

	return errors.New("import format must be one of CSV|TSV|FIXED|JSON|LTSV")
}

func (f *Flags) SetDelimiter(s string) error {
	if len(s) < 1 {
		return nil
	}

	delimiter, err := ParseDelimiter(s)
	if err != nil {
		return err
	}

	f.ImportOptions.Delimiter = delimiter
	return nil
}

func (f *Flags) SetDelimiterPositions(s string) error {
	if len(s) < 1 {
		return nil
	}
	delimiterPositions, singleLine, err := ParseDelimiterPositions(s)
	if err != nil {
		return err
	}

	f.ImportOptions.DelimiterPositions = delimiterPositions
	f.ImportOptions.SingleLine = singleLine
	return nil
}

func (f *Flags) SetJsonQuery(s string) {
	f.ImportOptions.JsonQuery = TrimSpace(s)
}

func (f *Flags) SetEncoding(s string) error {
	if len(s) < 1 {
		return nil
	}

	encoding, err := ParseEncoding(s)
	if err != nil {
		return err
	}

	f.ImportOptions.Encoding = encoding
	return nil
}

func (f *Flags) SetNoHeader(b bool) {
	f.ImportOptions.NoHeader = b
}

func (f *Flags) SetWithoutNull(b bool) {
	f.ImportOptions.WithoutNull = b
}

func (f *Flags) SetFormat(s string, outfile string) error {
	var fm Format
	var escape txjson.EscapeType
	var err error

	switch s {
	case "":
		switch strings.ToLower(filepath.Ext(outfile)) {
		case CsvExt:
			fm = CSV
		case TsvExt:
			fm = TSV
		case JsonExt:
			fm = JSON
		case LtsvExt:
			fm = LTSV
		case GfmExt:
			fm = GFM
		case OrgExt:
			fm = ORG
		default:
			return nil
		}
	default:
		if fm, escape, err = ParseFormat(s, f.ExportOptions.JsonEscape); err != nil {
			return err
		}
	}

	f.ExportOptions.Format = fm
	f.ExportOptions.JsonEscape = escape
	return nil
}

func (f *Flags) SetWriteEncoding(s string) error {
	if len(s) < 1 {
		return nil
	}

	encoding, err := ParseEncoding(s)
	if err != nil || encoding == text.AUTO {
		return errors.New("write-encoding must be one of UTF8|UTF8M|UTF16|UTF16BE|UTF16LE|UTF16BEM|UTF16LEM|SJIS")
	}

	f.ExportOptions.Encoding = encoding
	return nil
}

func (f *Flags) SetWriteDelimiter(s string) error {
	if len(s) < 1 {
		return nil
	}

	delimiter, err := ParseDelimiter(s)
	if err != nil {
		return errors.New("write-delimiter must be one character")
	}

	f.ExportOptions.Delimiter = delimiter
	return nil
}

func (f *Flags) SetWriteDelimiterPositions(s string) error {
	if len(s) < 1 {
		return nil
	}
	delimiterPositions, singleLine, err := ParseDelimiterPositions(s)
	if err != nil {
		return errors.New(fmt.Sprintf("write-delimiter-positions must be %q or a JSON array of integers", DelimitAutomatically))
	}

	f.ExportOptions.DelimiterPositions = delimiterPositions
	f.ExportOptions.SingleLine = singleLine
	return nil
}

func (f *Flags) SetWithoutHeader(b bool) {
	f.ExportOptions.WithoutHeader = b
}

func (f *Flags) SetLineBreak(s string) error {
	if len(s) < 1 {
		return nil
	}

	lb, err := ParseLineBreak(s)
	if err != nil {
		return err
	}

	f.ExportOptions.LineBreak = lb
	return nil
}

func (f *Flags) SetJsonEscape(s string) error {
	var escape txjson.EscapeType
	var err error

	if escape, err = ParseJsonEscapeType(s); err != nil {
		return err
	}

	f.ExportOptions.JsonEscape = escape
	return nil
}

func (f *Flags) SetPrettyPrint(b bool) {
	f.ExportOptions.PrettyPrint = b
}

func (f *Flags) SetStripEndingLineBreak(b bool) {
	f.ExportOptions.StripEndingLineBreak = b
}

func (f *Flags) SetEncloseAll(b bool) {
	f.ExportOptions.EncloseAll = b
}

func (f *Flags) SetColor(b bool) {
	f.ExportOptions.Color = b
}

func (f *Flags) SetEastAsianEncoding(b bool) {
	f.ExportOptions.EastAsianEncoding = b
}

func (f *Flags) SetCountDiacriticalSign(b bool) {
	f.ExportOptions.CountDiacriticalSign = b
}

func (f *Flags) SetCountFormatCode(b bool) {
	f.ExportOptions.CountFormatCode = b
}

func (f *Flags) SetQuiet(b bool) {
	f.Quiet = b
}

func (f *Flags) SetLimitRecursion(i int64) {
	if i < 0 {
		i = -1
	}
	f.LimitRecursion = i
}

func (f *Flags) SetCPU(i int) {
	if i < 1 {
		i = 1
	}

	if runtime.NumCPU() < i {
		i = runtime.NumCPU()
	}

	f.CPU = i
}

func (f *Flags) SetStats(b bool) {
	f.Stats = b
}
