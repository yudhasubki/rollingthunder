package postgres

var Types = map[string]string{
	// Character types
	"character":         "char",
	"character varying": "varchar",
	"varchar":           "varchar",
	"text":              "text",

	// Numeric types
	"smallint":         "int2",
	"integer":          "int4",
	"bigint":           "int8",
	"decimal":          "decimal",
	"numeric":          "decimal",
	"real":             "float4",
	"double precision": "float8",
	"serial":           "serial4",
	"bigserial":        "serial8",
	"smallserial":      "serial2",

	// Date/time types
	"timestamp without time zone": "timestamp",
	"timestamp with time zone":    "timestamptz",
	"time without time zone":      "time",
	"time with time zone":         "timetz",
	"date":                        "date",
	"interval":                    "interval",

	// Boolean
	"boolean": "bool",

	// UUID
	"uuid": "uuid",

	// JSON
	"json":  "json",
	"jsonb": "jsonb",

	// Binary
	"bytea": "bytea",

	// Geometric types
	"point":   "point",
	"line":    "line",
	"lseg":    "lseg",
	"box":     "box",
	"path":    "path",
	"polygon": "polygon",
	"circle":  "circle",

	// Network
	"cidr":     "cidr",
	"inet":     "inet",
	"macaddr":  "macaddr",
	"macaddr8": "macaddr8",

	// Arrays
	"ARRAY": "array",

	// Range types
	"int4range": "int4range",
	"int8range": "int8range",
	"numrange":  "numrange",
	"tsrange":   "tsrange",
	"tstzrange": "tstzrange",
	"daterange": "daterange",

	// Others
	"money":       "money",
	"bit":         "bit",
	"bit varying": "varbit",
	"tsvector":    "tsvector",
	"tsquery":     "tsquery",
	"xml":         "xml",
	"oid":         "oid",
}
