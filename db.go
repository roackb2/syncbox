package syncbox

import (
	"database/sql"
	"fmt"
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
	ID     int    `mysql:"id,pk,auto_increment"`
	FileID int    `mysql:"file_id,fk=file.id"`
	Path   string `mysql:"path"`
	Device string `mysql:"device"`
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
	Conn   *sql.DB
	Tables []reflect.Value
}

// NewDB instantiates a DB instance
func NewDB(tables ...interface{}) (*DB, error) {
	db := &DB{
		DBConfig: AWSDBConfig,
		Logger:   NewLogger(DefaultAppPrefix, GlobalLogInfo, GlobalLogError, GlobalLogDebug),
	}
	conn, err := sql.Open("mysql", db.User+":"+db.Password+"@tcp("+db.Host+":"+db.Port+")/"+db.Database+"?charset=utf8")
	if err != nil {
		db.Logger.LogDebug("error on new db: %v\n", err)
		return nil, err
	}
	db.Conn = conn
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
	stmt, err := db.Conn.Prepare(query)
	if err != nil {
		db.LogDebug("error preparing statment: %v\n", err)
		return nil, err
	}
	var res sql.Result
	if len(args) == 0 || (len(args) == 1 && args[0] == nil) {
		res, err = stmt.Exec()
	} else {
		res, err = stmt.Exec(args)
	}
	if err != nil {
		db.LogDebug("error executing statement: %v\n", err)
		return nil, err
	}
	return res, nil
}

func createSchema(table interface{}) (string, error) {
	stat := "CREATE TABLE IF NOT EXISTS "
	structType := reflect.TypeOf(table)
	structName := strings.Split(structType.String(), ".")[1]
	tableName := underscore(strings.Replace(structName, "Table", "", 1))
	stat += tableName + "( "
	fieldNum := structType.NumField()
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
			case opt == "pk":
				stat += "PRIMARY KEY "
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
	for _, constraint := range constraints {
		stat += constraint + ", "
	}
	stat = strings.TrimRight(stat, " ")
	stat = strings.TrimRight(stat, ",")
	stat += ")"
	return stat, nil
}

func test() {
	db, err := sql.Open("mysql", "astaxie:astaxie@/test?charset=utf8")
	checkErr(err)

	// insert data
	stmt, err := db.Prepare("INSERT userinfo SET username=?,departname=?,created=?")
	checkErr(err)

	res, err := stmt.Exec("astaxie", "研发部门", "2012-12-09")
	checkErr(err)

	id, err := res.LastInsertId()
	checkErr(err)

	fmt.Println(id)
	// update data
	stmt, err = db.Prepare("update userinfo set username=? where uid=?")
	checkErr(err)

	res, err = stmt.Exec("astaxieupdate", id)
	checkErr(err)

	affect, err := res.RowsAffected()
	checkErr(err)

	fmt.Println(affect)

	// query data
	rows, err := db.Query("SELECT * FROM userinfo")
	checkErr(err)

	for rows.Next() {
		var uid int
		var username string
		var department string
		var created string
		err = rows.Scan(&uid, &username, &department, &created)
		checkErr(err)
		fmt.Println(uid)
		fmt.Println(username)
		fmt.Println(department)
		fmt.Println(created)
	}

	// delelte data
	stmt, err = db.Prepare("delete from userinfo where uid=?")
	checkErr(err)

	res, err = stmt.Exec(id)
	checkErr(err)

	affect, err = res.RowsAffected()
	checkErr(err)

	fmt.Println(affect)

	db.Close()

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
