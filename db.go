package syncbox

import (
	"database/sql"
	"os"
	"reflect"
	"regexp"
	"strings"

	// imort as driver
	_ "github.com/go-sql-driver/mysql"
)

// constants for DB connections
const (
	TableNamingSuffix = "Table"
)

// config varaibles and others
var (
	camel        = regexp.MustCompile("(^[^A-Z]*|[A-Z]*)([A-Z][^A-Z]+|$)")
	LocalDBConfg = &DBConfig{
		User:     "syncbox",
		Password: "syncbox",
		Host:     "localhost",
		Port:     "3306",
		Database: "syncbox",
	}

	AWSDBConfig = &DBConfig{
		User:     os.Getenv("SB_DB_USER"),
		Password: os.Getenv("SB_DB_PWD"),
		Host:     os.Getenv("SB_DB_HOST"),
		Port:     os.Getenv("SB_DB_PORT"),
		Database: os.Getenv("SB_DB_DATABASE"),
	}
)

// UserTable stores information about users
type UserTable struct {
	ID       int    `mysql:"id,pk,auto_increment"`
	Username string `mysql:"username,unique"`
	Password string `mysql:"password"`
}

// FileTable stores information about files
type FileTable struct {
	ID       int    `mysql:"id,pk,auto_increment"`
	Checksum string `mysql:"checksum,unique"`
	UserID   int    `mysql:"user_id,fk=user.id"`
}

// FileRefTable stores edges that connects from file tree nodes to files
type FileRefTable struct {
	ID     int    `mysql:"id,unique,auto_increment"`
	UserID int    `mysql:"user_id,pk,fk=user.id"`
	FileID int    `mysql:"file_id,pk,fk=file.id"`
	Path   string `mysql:"path,pk"`
	Device string `mysql:"device,pk"`
}

// DBConfig is the structure for DB configurations
type DBConfig struct {
	User     string
	Password string
	Host     string
	Port     string
	Database string
}

// DB is the database
type DB struct {
	*DBConfig
	*Logger
	*sql.DB
	Tables []reflect.Value
}

// NewDB instantiates a DB instance
func NewDB(tables ...interface{}) (*DB, error) {
	db := &DB{
		DBConfig: AWSDBConfig,
		Logger:   NewDefaultLogger(),
	}
	conn, err := sql.Open("mysql", db.User+":"+db.Password+"@tcp("+db.Host+":"+db.Port+")/"+db.Database+"?charset=utf8")
	if err != nil {
		db.Logger.LogDebug("error on new db: %v\n", err)
		return nil, err
	}
	db.DB = conn
	for _, table := range tables {
		tableType := reflect.ValueOf(table)
		db.Tables = append(db.Tables, tableType)
		db.Exec(createSchema(table))
		if err != nil {
			db.Logger.LogDebug("error on execute statement: %v\n", err)
			return nil, err
		}
	}
	return db, nil
}

// Exec executes raw SQL commands
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	stmt, err := db.Prepare(query)
	if err != nil {
		db.LogDebug("error preparing statment: %v\n", err)
		return nil, err
	}
	var res sql.Result
	if len(args) == 0 || (len(args) == 1 && args[0] == nil) {
		res, err = stmt.Exec()
	} else {
		res, err = stmt.Exec(args...)
	}
	if err != nil {
		db.LogDebug("error executing statement: %v\n", err)
		return nil, err
	}
	return res, nil
}

// Query is a structure type that forms a query with builder pattern
type Query struct {
	SelectClause string
	FromClause   string
	WhereClause  string
	db           *DB
}

// NewQuery instantiates a query
func NewQuery(db *DB) *Query {
	return &Query{
		db:           db,
		SelectClause: "SELECT ",
		FromClause:   " FROM ",
		WhereClause:  " WHERE ",
	}
}

func (q *Query) copy(q2 *Query) {
	q1Val := reflect.ValueOf(q).Elem()
	q2Val := reflect.ValueOf(q2).Elem()
	for i := 0; i < q1Val.NumField(); i++ {
		q1Field := q1Val.Field(i)
		q2Field := q2Val.Field(i)
		switch q2Field.Kind() {
		case reflect.String:
			q1Field.SetString(q2Field.String())
		case reflect.Int:
			q1Field.SetInt(q2Field.Int())
		}
	}
}

// Select forms the select clause with the constraint
func (q *Query) Select(condition string) *Query {
	newQ := NewQuery(q.db)
	newQ.copy(q)
	newQ.SelectClause += condition
	return newQ
}

// From forms the from clause according to the input
func (q *Query) From(condition string) *Query {
	newQ := NewQuery(q.db)
	newQ.copy(q)
	newQ.FromClause += condition
	return newQ
}

// Where forms the where clause acoording to the input
func (q *Query) Where(condition string) *Query {
	newQ := NewQuery(q.db)
	newQ.copy(q)
	newQ.WhereClause += condition
	return newQ
}

// Exec executes the query
func (q *Query) Exec() (*sql.Rows, error) {
	stmt := q.SelectClause + q.FromClause + q.WhereClause
	rows, err := q.db.Query(stmt)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Populate populates data to a slice of reference of tables
// the input should be address of slice of pointers of certain table
func (q *Query) Populate(a interface{}) error {
	sliceVal := reflect.ValueOf(a).Elem()
	sliceType := reflect.TypeOf(a).Elem()
	elemPtr := sliceType.Elem()
	elemType := elemPtr.Elem()
	rows, err := q.Exec()
	if err != nil {
		return err
	}
	records := reflect.MakeSlice(reflect.SliceOf(elemPtr), 0, 0)
	for rows.Next() {
		recordPtr := reflect.New(elemType)
		record := recordPtr.Elem()
		fieldNum := elemType.NumField()
		fieldList := make([]interface{}, 0, 0)
		for i := 0; i < fieldNum; i++ {
			field := record.Field(i)
			switch field.Kind() {
			case reflect.String:
				temp := ""
				fieldList = append(fieldList, &temp)
			case reflect.Int:
				temp := 0
				fieldList = append(fieldList, &temp)
			}
		}
		rows.Scan(fieldList...)
		for i := 0; i < fieldNum; i++ {
			field := record.Field(i)
			switch field.Kind() {
			case reflect.String:
				field.Set(reflect.ValueOf(*fieldList[i].(*string)))
			case reflect.Int:
				field.Set(reflect.ValueOf(*fieldList[i].(*int)))
			}
		}
		records = reflect.Append(records, recordPtr)
	}
	sliceVal.Set(records)
	return nil
}

func createSchema(table interface{}) (string, error) {
	stat := "CREATE TABLE IF NOT EXISTS "
	structType := reflect.TypeOf(table)
	structName := strings.Split(structType.String(), ".")[1]
	tableName := underscore(strings.Replace(structName, "Table", "", 1))
	stat += tableName + "( "
	fieldNum := structType.NumField()
	var primaryKeys []string
	var constraints []string
	for i := 0; i < fieldNum; i++ {
		var fieldName string
		field := structType.Field(i)
		tag := field.Tag.Get("mysql")
		opts := strings.Split(tag, ",")
		if len(opts) > 0 {
			fieldName = opts[0]
			opts = opts[1:len(opts)]
		} else {
			fieldName = field.Name
		}
		stat += fieldName + " "
		fieldType := field.Type
		switch fieldType.Kind() {
		case reflect.Int:
			stat += "INT "
		case reflect.String:
			stat += "VARCHAR(255) "
		}
		for _, opt := range opts {
			switch {
			case opt == "key":
				stat += "KEY "
			case opt == "pk":
				primaryKeys = append(primaryKeys, fieldName)
			case opt == "auto_increment":
				stat += "AUTO_INCREMENT "
			case opt == "unique":
				stat += "UNIQUE "
			case strings.HasPrefix(opt, "fk"):
				fkRef := strings.Replace(opt, "fk=", "", 1)
				splitted := strings.Split(fkRef, ".")
				table := splitted[0]
				column := splitted[1]
				constraint := "FOREIGN KEY (" + fieldName + ")" + " REFERENCES " + table + "(" + column + ")"
				constraints = append(constraints, constraint)
			}
		}
		stat = strings.TrimRight(stat, " ")
		stat += ", "
	}
	stat += "PRIMARY KEY ("
	for _, pk := range primaryKeys {
		stat += pk + ", "
	}
	stat = strings.TrimRight(stat, ", ")
	stat += "), "

	for _, constraint := range constraints {
		stat += constraint + ", "
	}
	stat = strings.TrimRight(stat, ", ")
	stat += ")"
	return stat, nil
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func underscore(s string) string {
	var a []string
	for _, sub := range camel.FindAllStringSubmatch(s, -1) {
		if sub[1] != "" {
			a = append(a, sub[1])
		}
		if sub[2] != "" {
			a = append(a, sub[2])
		}
	}
	return strings.ToLower(strings.Join(a, "_"))
}
