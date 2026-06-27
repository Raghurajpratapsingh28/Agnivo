package repository

import (
	"context"
	"strconv"
	"strings"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/jackc/pgx/v5"
)

// BulkInsert performs a high-throughput insert of many rows via the PostgreSQL
// COPY protocol, which is dramatically faster than individual INSERTs for large
// batches. columns names the target columns; each row in rows must provide
// values in the same order. It returns the number of rows inserted.
//
// COPY does not run per-row triggers the same way as INSERT in all cases and
// does not support ON CONFLICT; use BulkUpsert when conflict handling is needed.
func (r *Repository[T]) BulkInsert(ctx context.Context, columns []string, rows [][]any) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	n, err := r.db.Conn(ctx).CopyFrom(ctx,
		pgx.Identifier{r.table}, columns, pgx.CopyFromRows(rows))
	if err != nil {
		return 0, postgres.Translate(err, "repository: bulk insert")
	}
	return n, nil
}

// BulkUpsert inserts many rows in one multi-row INSERT statement with an
// ON CONFLICT clause, updating the listed columns on conflict. conflictTarget is
// the conflict column(s) (e.g. "id" or "(tenant_id, slug)"). updateColumns are
// the columns to overwrite with the proposed values on conflict; an empty slice
// performs DO NOTHING. Returns the number of rows affected.
func (r *Repository[T]) BulkUpsert(ctx context.Context, columns []string, rows [][]any, conflictTarget string, updateColumns []string) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(r.table)
	b.WriteString(" (")
	b.WriteString(strings.Join(columns, ", "))
	b.WriteString(") VALUES ")

	args := make([]any, 0, len(rows)*len(columns))
	pos := 1
	for ri, row := range rows {
		if ri > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		for ci := range columns {
			if ci > 0 {
				b.WriteString(", ")
			}
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(pos))
			pos++
			args = append(args, row[ci])
		}
		b.WriteByte(')')
	}

	b.WriteString(" ON CONFLICT ")
	b.WriteString(conflictTarget)
	if len(updateColumns) == 0 {
		b.WriteString(" DO NOTHING")
	} else {
		b.WriteString(" DO UPDATE SET ")
		for i, col := range updateColumns {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(col)
			b.WriteString(" = EXCLUDED.")
			b.WriteString(col)
		}
	}

	tag, err := r.db.Conn(ctx).Exec(ctx, b.String(), args...)
	if err != nil {
		return 0, postgres.Translate(err, "repository: bulk upsert")
	}
	return tag.RowsAffected(), nil
}

// BulkUpdate applies the same column→value map to many rows identified by their
// primary keys, in a single statement using a VALUES list join. It returns the
// number of rows updated. For per-row distinct values, build a CTE upstream.
func (r *Repository[T]) BulkUpdateByIDs(ctx context.Context, ids []any, values map[string]any) (int64, error) {
	if len(ids) == 0 || len(values) == 0 {
		return 0, nil
	}
	setClause, args := buildSet(values, 0)
	idCond := In(r.pk, ids)
	where, idArgs := idCond.build(len(args) + 1)
	args = append(args, idArgs...)

	sql := "UPDATE " + r.table + " SET " + setClause + " WHERE " + where
	if r.softDeleteCol != "" {
		sql += " AND " + r.softDeleteCol + " IS NULL"
	}
	tag, err := r.db.Conn(ctx).Exec(ctx, sql, args...)
	if err != nil {
		return 0, postgres.Translate(err, "repository: bulk update")
	}
	return tag.RowsAffected(), nil
}
