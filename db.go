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

// Table is the general table interface that specifies what tables should implement
type Table interface {
	CreateSchema() string
}

// UserTable stores information about users
type UserTable struct {
	ID       int    `mysql:"id,pk,auto_increment"`
	Username string `mysql:"username"`
	Password string `mysql:"password"`
}

// CreateSchema implements Table interface to create schema
func (ur *UserTable) CreateSchema() string {
	return `CREATE TABLE IF NOT EXISTS user (
				id INT PRIMARY KEY AUTO_INCREMENT,
				username VARCHAR(255),
				password VARCHAR(255)
			)`
}

// FileTable stores information about files
type FileTable struct {
	ID       int    `mysql:"id,pk,auto_increment"`
	Checksum string `mysql:"checksum"`
	UserID   int    `mysql:"user_id,fk=user.id"`
}

// CreateSchema implements Table interface to create schema
func (fr *FileTable) CreateSchema() string {
	return `CREATE TABLE IF NOT EXISTS file (
				id INT PRIMARY KEY AUTO_INCREMENT,
				checksum VARCHAR(255),
				user_id INT,
				FOREIGN KEY (user_id) REFERENCES user(id)
			)`
}

// FileRefTable stores edges that connects from file tree nodes to files
type FileRefTable struct {
	ID     int    `mysql:"id,pk,auto_increment"`
	FileID int    `mysql:"file_id,fk=file.id"`
	Path   string `mysql:"path"`
	Device string `mysql:"device"`
}

// CreateSchema implements Table interface to create schema
func (frr *FileRefTable) CreateSchema() string {
	return `CREATE TABLE IF NOT EXISTS file_ref (
				id INT PRIMARY KEY AUTO_INCREMENT,
				file_id INT,
				path VARCHAR(255),
				device VARCHAR(255),
				FOREIGN KEY (file_id) REFERENCES file(id)
			)`
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
func NewDB(tables ...Table) (*DB, error) {
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
		db.Exec(table.CreateSchema())
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
	if len(args) == 0 {
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

// func createSchema(table interface{}) (string, error) {
// 	stat := "CREATE TABLE IF NOT EXISTS "
// 	structType := reflect.TypeOf(table)
// 	stat += underscore(strings.Trim(structType.Name(), "Record")) + "( "
// 	fieldNum := structType.NumField()
// 	for i := 0; i < fieldNum; i++ {
// 		var fieldName string
// 		field := structType.Field(i)
// 		tag := field.Tag.Get("mysql")
// 		opts := strings.Split(tag, ",")
// 		if len(opts) > 0 {
// 			fieldName = opts[0]
// 			opts = opts[1 : len(opts)-1]
// 		} else {
// 			fieldName = field.Name
// 		}
// 		stat += fieldName + " "
// 		for _, opt := range opts {
// 			if opt == "pk" {
//
// 			}
// 		}
// 	}
// 	return stat, nil
// }

// func (db *DB) CreateTable() error {
//
// }

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
