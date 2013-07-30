package sqlscan

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Db is a generic database interface, matching both *sql.Db and *sql.Tx
type Db interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// Load loads a record using a query for the primary key field.
// Returns sql.ErrNoRows if not found.
func Load(db Db, table string, pk int, dst interface{}) error {
	columns, err := ColumnsQuoted(true, dst)
	if err != nil {
		return err
	}

	// make sure we have a primary key field
	pkName, _, err := PrimaryKey(dst)
	if err != nil {
		return err
	}
	if pkName == "" {
		return fmt.Errorf("sqlscan.Load: no primary key field found")
	}

	// run the query
	q := fmt.Sprintf("SELECT %s FROM %s WHERE %s = %s", columns, Quote+table+Quote, pkName, Placeholder)

	rows, err := db.Query(q, pk)
	if err != nil {
		return fmt.Errorf("sqlscan.Load: DB error in Query: %v", err)
	}

	// scan the row
	return ScanRow(rows, dst)
}

// Insert performs an INSERT query for the given record.
// If the record has a primary key flagged, it must be zero, and it
// will be set to the newly-allocated primary key value from the database
// as returned by LastInsertId.
func Insert(db Db, table string, src interface{}) error {
	pkName, pkValue, err := PrimaryKey(src)
	if err != nil {
		return err
	}
	if pkName != "" && pkValue != 0 {
		return fmt.Errorf("sqlscan.Insert: primary key must be zero")
	}

	// gather the query parts
	namesPart, err := ColumnsQuoted(false, src)
	if err != nil {
		return err
	}
	valuesPart, err := SavePlaceholdersString(false, src)
	if err != nil {
		return err
	}
	values, err := SaveValues(false, src)
	if err != nil {
		return err
	}

	// run the query
	q := fmt.Sprintf("INSERT INTO %s%s%s (%s) VALUES (%s)", Quote, table, Quote,
		namesPart, valuesPart)
	if PostgreSQL && pkName != "" {
		q += " RETURNING " + Quote + pkName + Quote
		var newPk int
		err := db.QueryRow(q, values...).Scan(&newPk)
		if err != nil {
			return fmt.Errorf("sqlscan.Insert: DB error in QueryRow: %v", err)
		}
		if err = SetPrimaryKey(newPk, src); err != nil {
			return fmt.Errorf("sqlscan.Insert: Error saving updated pk: %v", err)
		}
	} else if pkName != "" {
		result, err := db.Exec(q, values...)
		if err != nil {
			return fmt.Errorf("sqlscan.Insert: DB error in Exec: %v", err)
		}

		// save the new primary key
		newPk, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("sqlscan.Insert: DB error getting new primary key value: %v", err)
		}
		if err = SetPrimaryKey(int(newPk), src); err != nil {
			return fmt.Errorf("sqlscan.Insert: Error saving updated pk: %v", err)
		}
	} else {
		// no primary key, so no need to lookup new value
		_, err := db.Exec(q, values...)
		if err != nil {
			return fmt.Errorf("sqlscan.Insert: DB error in Exec: %v", err)
		}
	}

	return nil
}

// Update performs and UPDATE query for the given record.
// The record must have an integer primary key field that is non-zero,
// and it will be used to select the database row that gets updated.
func Update(db Db, table string, src interface{}) error {
	// gather the query parts
	names, err := Columns(false, src)
	if err != nil {
		return err
	}
	placeholders, err := SavePlaceholders(false, src)
	if err != nil {
		return err
	}
	values, err := SaveValues(false, src)
	if err != nil {
		return err
	}

	// form the column=placeholder pairs
	var pairs []string
	for i := 0; i < len(names) && i < len(placeholders); i++ {
		pair := fmt.Sprintf("%s%s%s=%s", Quote, names[i], Quote, placeholders[i])
		pairs = append(pairs, pair)
	}

	pkName, pkValue, err := PrimaryKey(src)
	if err != nil {
		return err
	}
	if pkName == "" {
		return fmt.Errorf("sqlscan.Update: no primary key field")
	}
	if pkValue < 1 {
		return fmt.Errorf("sqlscan.Update: primary key must be an integer > 0")
	}
	ph := strings.Replace(Placeholder, "1", strconv.FormatInt(int64(len(placeholders)+1), 10), 1)

	// run the query
	q := fmt.Sprintf("UPDATE %s%s%s SET %s WHERE %s%s%s=%s", Quote, table, Quote,
		strings.Join(pairs, ","),
		Quote, pkName, Quote, ph)
	values = append(values, pkValue)

	if _, err := db.Exec(q, values...); err != nil {
		return fmt.Errorf("sqlscan.Update: DB error in Exec: %v", err)
	}

	return nil
}

// Save performs an INSERT or an UPDATE, depending on whether or not
// a primary keys exists and is non-zero.
func Save(db Db, table string, src interface{}) error {
	pkName, pkValue, err := PrimaryKey(src)
	if err != nil {
		return err
	}
	if pkName != "" && pkValue != 0 {
		return Update(db, table, src)
	} else {
		return Insert(db, table, src)
	}
}

// QueryOne performs the given query with the given arguments, scanning a
// single row of results into dst. Returns sql.ErrNoRows if there was no
// result row.
func QueryRow(db Db, dst interface{}, query string, args ...interface{}) error {
	// perform the query
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}

	// gather the result
	return ScanRow(rows, dst)
}

// QueryAll performs the given query with the given arguments, scanning
// all results rows into dst.
func QueryAll(db Db, dst interface{}, query string, args ...interface{}) error {
	// perform the query
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}

	// gather the results
	return ScanAll(rows, dst)
}
