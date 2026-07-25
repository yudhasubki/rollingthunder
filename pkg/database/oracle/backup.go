package oracle

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"rollingthunder/pkg/database"

	go_ora "github.com/sijms/go-ora/v2"
)

const (
	oracleDataPumpChunkSize      = 1024 * 1024
	oracleDataPumpUploadChunk    = 32767
	oracleDataPumpLogLimit       = 32 * 1024
	oracleDataPumpCleanupTimeout = 30 * time.Second

	oracleBackupDirectoriesQuery = `
		SELECT directory_name, directory_path
		FROM all_directories
		ORDER BY
			CASE WHEN directory_name = 'DATA_PUMP_DIR' THEN 0 ELSE 1 END,
			directory_name`

	oracleDataPumpExportBlock = `
		DECLARE
			job_handle NUMBER := NULL;
			job_state VARCHAR2(30);
		BEGIN
			job_handle := DBMS_DATAPUMP.OPEN(
				operation => 'EXPORT',
				job_mode => 'SCHEMA',
				job_name => :job_name,
				version => 'COMPATIBLE'
			);
			DBMS_DATAPUMP.ADD_FILE(
				handle => job_handle,
				filename => :dump_file,
				directory => :directory_name,
				filetype => DBMS_DATAPUMP.KU$_FILE_TYPE_DUMP_FILE,
				reusefile => 1
			);
			DBMS_DATAPUMP.ADD_FILE(
				handle => job_handle,
				filename => :log_file,
				directory => :directory_name,
				filetype => DBMS_DATAPUMP.KU$_FILE_TYPE_LOG_FILE,
				reusefile => 1
			);
			DBMS_DATAPUMP.METADATA_FILTER(
				handle => job_handle,
				name => 'SCHEMA_EXPR',
				value => :schema_expression
			);
			DBMS_DATAPUMP.SET_PARAMETER(
				handle => job_handle,
				name => 'METRICS',
				value => 1
			);
			DBMS_DATAPUMP.START_JOB(job_handle);
			DBMS_DATAPUMP.WAIT_FOR_JOB(job_handle, job_state);
			:job_state := job_state;
		EXCEPTION
			WHEN OTHERS THEN
				IF job_handle IS NOT NULL THEN
					BEGIN
						DBMS_DATAPUMP.DETACH(job_handle);
					EXCEPTION
						WHEN OTHERS THEN NULL;
					END;
				END IF;
				RAISE;
		END;`

	oracleDataPumpImportBlock = `
		DECLARE
			job_handle NUMBER := NULL;
			job_state VARCHAR2(30);
		BEGIN
			job_handle := DBMS_DATAPUMP.OPEN(
				operation => 'IMPORT',
				job_mode => 'SCHEMA',
				job_name => :job_name,
				version => 'COMPATIBLE'
			);
			DBMS_DATAPUMP.ADD_FILE(
				handle => job_handle,
				filename => :dump_file,
				directory => :directory_name,
				filetype => DBMS_DATAPUMP.KU$_FILE_TYPE_DUMP_FILE
			);
			DBMS_DATAPUMP.ADD_FILE(
				handle => job_handle,
				filename => :log_file,
				directory => :directory_name,
				filetype => DBMS_DATAPUMP.KU$_FILE_TYPE_LOG_FILE,
				reusefile => 1
			);
			DBMS_DATAPUMP.METADATA_FILTER(
				handle => job_handle,
				name => 'SCHEMA_EXPR',
				value => :schema_expression
			);
			DBMS_DATAPUMP.SET_PARAMETER(
				handle => job_handle,
				name => 'TABLE_EXISTS_ACTION',
				value => 'REPLACE'
			);
			DBMS_DATAPUMP.SET_PARAMETER(
				handle => job_handle,
				name => 'METRICS',
				value => 1
			);
			DBMS_DATAPUMP.START_JOB(job_handle);
			DBMS_DATAPUMP.WAIT_FOR_JOB(job_handle, job_state);
			:job_state := job_state;
		EXCEPTION
			WHEN OTHERS THEN
				IF job_handle IS NOT NULL THEN
					BEGIN
						DBMS_DATAPUMP.DETACH(job_handle);
					EXCEPTION
						WHEN OTHERS THEN NULL;
					END;
				END IF;
				RAISE;
		END;`

	oracleDataPumpStopBlock = `
		DECLARE
			job_handle NUMBER := NULL;
		BEGIN
			job_handle := DBMS_DATAPUMP.ATTACH(job_name => :job_name);
			DBMS_DATAPUMP.STOP_JOB(
				handle => job_handle,
				immediate => 1,
				keep_master => 0
			);
		EXCEPTION
			WHEN OTHERS THEN NULL;
		END;`

	oracleServerFileWriteBlock = `
		DECLARE
			file_handle UTL_FILE.FILE_TYPE;
		BEGIN
			file_handle := UTL_FILE.FOPEN(
				:directory_name,
				:file_name,
				:open_mode,
				32767
			);
			UTL_FILE.PUT_RAW(file_handle, :file_data, TRUE);
			UTL_FILE.FCLOSE(file_handle);
		EXCEPTION
			WHEN OTHERS THEN
				IF UTL_FILE.IS_OPEN(file_handle) THEN
					UTL_FILE.FCLOSE(file_handle);
				END IF;
				RAISE;
		END;`

	oracleServerFileRemoveBlock = `
		BEGIN
			UTL_FILE.FREMOVE(:directory_name, :file_name);
		EXCEPTION
			WHEN OTHERS THEN NULL;
		END;`
)

func oracleDataPumpNames(operation string) (string, string, string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", "", "", fmt.Errorf("generate Oracle Data Pump job ID: %w", err)
	}
	token := strings.ToUpper(hex.EncodeToString(random))
	operation = strings.ToUpper(strings.TrimSpace(operation))
	if operation != "EXP" && operation != "IMP" {
		return "", "", "", fmt.Errorf(
			"unsupported Oracle Data Pump operation %q",
			operation,
		)
	}
	job := "RT_" + operation + "_" + token
	base := strings.ToLower(job)
	return job, base + ".dmp", base + ".log", nil
}

func oracleDataPumpSchemaExpression(schema string) string {
	return "IN ('" + strings.ReplaceAll(schema, "'", "''") + "')"
}

func (o *Oracle) resolveDataPumpSchema(
	ctx context.Context,
	value string,
) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = o.defaultSchema("")
	}
	if value == "" || len(value) > 128 ||
		strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("a valid Oracle application schema is required")
	}
	var (
		schema           string
		oracleMaintained string
	)
	if err := o.conn.QueryRowContext(
		ctx,
		`SELECT username, oracle_maintained
		 FROM (
			SELECT username, oracle_maintained
			FROM all_users
			WHERE username = :schema_name
				OR username = UPPER(:schema_name)
			ORDER BY CASE WHEN username = :schema_name THEN 0 ELSE 1 END
		 )
		 WHERE ROWNUM = 1`,
		sql.Named("schema_name", value),
	).Scan(&schema, &oracleMaintained); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("Oracle schema %q does not exist", value)
		}
		return "", fmt.Errorf("inspect Oracle schema %q: %w", value, err)
	}
	if strings.EqualFold(oracleMaintained, "Y") {
		return "", fmt.Errorf(
			"Oracle-maintained schema %q cannot be backed up or restored through Rolling Thunder",
			schema,
		)
	}
	return schema, nil
}

func (o *Oracle) resolveBackupDirectory(
	ctx context.Context,
	value string,
) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 ||
		strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("an Oracle Data Pump directory is required")
	}
	var directory string
	if err := o.conn.QueryRowContext(
		ctx,
		`SELECT directory_name
		 FROM all_directories
		 WHERE directory_name = :1 OR directory_name = UPPER(:1)
		 ORDER BY CASE WHEN directory_name = :1 THEN 0 ELSE 1 END
		 FETCH FIRST 1 ROW ONLY`,
		value,
	).Scan(&directory); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf(
				"Oracle Data Pump directory %q is not accessible",
				value,
			)
		}
		return "", fmt.Errorf("inspect Oracle Data Pump directory: %w", err)
	}
	return directory, nil
}

func (o *Oracle) GetBackupDirectories(
	ctx context.Context,
) ([]database.BackupDirectory, error) {
	if err := o.ensureConnected(); err != nil {
		return nil, err
	}
	rows, err := o.conn.QueryContext(ctx, oracleBackupDirectoriesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	directories := make([]database.BackupDirectory, 0)
	for rows.Next() {
		var directory database.BackupDirectory
		if err := rows.Scan(&directory.Name, &directory.Path); err != nil {
			return nil, err
		}
		directories = append(directories, directory)
	}
	return directories, rows.Err()
}

func (o *Oracle) runDataPump(
	ctx context.Context,
	block string,
	jobName string,
	dumpFile string,
	logFile string,
	directory string,
	schema string,
) (string, error) {
	var state string
	_, err := o.conn.ExecContext(
		ctx,
		block,
		sql.Named("job_name", jobName),
		sql.Named("dump_file", dumpFile),
		sql.Named("log_file", logFile),
		sql.Named("directory_name", directory),
		sql.Named(
			"schema_expression",
			oracleDataPumpSchemaExpression(schema),
		),
		sql.Named(
			"job_state",
			go_ora.Out{Dest: &state, Size: 30},
		),
	)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	state = strings.ToUpper(strings.TrimSpace(state))
	if state != "COMPLETED" {
		return state, fmt.Errorf(
			"Oracle Data Pump job %s finished in state %q",
			jobName,
			state,
		)
	}
	return state, nil
}

func (o *Oracle) stopDataPumpJob(ctx context.Context, jobName string) {
	_, _ = o.conn.ExecContext(
		ctx,
		oracleDataPumpStopBlock,
		sql.Named("job_name", jobName),
	)
}

func (o *Oracle) removeServerFile(
	ctx context.Context,
	directory string,
	fileName string,
) {
	_, _ = o.conn.ExecContext(
		ctx,
		oracleServerFileRemoveBlock,
		sql.Named("directory_name", directory),
		sql.Named("file_name", fileName),
	)
}

func (o *Oracle) copyServerFile(
	ctx context.Context,
	writer io.Writer,
	directory string,
	fileName string,
	start int64,
	limit int64,
) error {
	var (
		file   go_ora.BFile
		length int64
	)
	if err := o.conn.QueryRowContext(
		ctx,
		`SELECT server_file, DBMS_LOB.GETLENGTH(server_file)
		 FROM (
			SELECT BFILENAME(:1, :2) AS server_file
			FROM dual
		 )`,
		directory,
		fileName,
	).Scan(&file, &length); err != nil {
		return fmt.Errorf("open Oracle server file locator: %w", err)
	}
	if err := file.OpenContext(ctx); err != nil {
		return fmt.Errorf("open Oracle server file: %w", err)
	}
	defer file.Close()
	if start < 0 {
		start = 0
	}
	if start > length {
		start = length
	}
	remaining := length - start
	if limit >= 0 && remaining > limit {
		remaining = limit
	}
	for offset := start; remaining > 0; {
		if err := ctx.Err(); err != nil {
			return err
		}
		count := int64(oracleDataPumpChunkSize)
		if count > remaining {
			count = remaining
		}
		chunk, err := file.ReadBytesFromPosContext(ctx, offset, count)
		if err != nil {
			return fmt.Errorf("read Oracle server file: %w", err)
		}
		if len(chunk) == 0 {
			return fmt.Errorf(
				"Oracle server file ended before its reported size",
			)
		}
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
		if _, err := writer.Write(chunk); err != nil {
			return err
		}
		offset += int64(len(chunk))
		remaining -= int64(len(chunk))
	}
	return nil
}

func (o *Oracle) writeServerFile(
	ctx context.Context,
	reader io.Reader,
	directory string,
	fileName string,
) error {
	buffer := make([]byte, oracleDataPumpUploadChunk)
	mode := "wb"
	written := int64(0)
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			chunk := append([]byte(nil), buffer[:count]...)
			if _, err := o.conn.ExecContext(
				ctx,
				oracleServerFileWriteBlock,
				sql.Named("directory_name", directory),
				sql.Named("file_name", fileName),
				sql.Named("open_mode", mode),
				sql.Named("file_data", chunk),
			); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("upload Oracle Data Pump file: %w", err)
			}
			mode = "ab"
			written += int64(count)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
		if count == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}
	if written == 0 {
		return fmt.Errorf("Oracle Data Pump restore file is empty")
	}
	return nil
}

func (o *Oracle) dataPumpFailure(
	operation string,
	jobName string,
	directory string,
	logFile string,
	cause error,
) error {
	cleanupCtx, cancel := context.WithTimeout(
		context.Background(),
		oracleDataPumpCleanupTimeout,
	)
	defer cancel()
	o.stopDataPumpJob(cleanupCtx, jobName)

	var log bytes.Buffer
	_ = o.copyServerFile(
		cleanupCtx,
		&log,
		directory,
		logFile,
		0,
		oracleDataPumpLogLimit,
	)
	detail := strings.TrimSpace(log.String())
	hint := oracleDataPumpFailureHint(cause, detail)
	if detail == "" {
		if hint != "" {
			return fmt.Errorf(
				"Oracle Data Pump %s failed: %w; %s",
				operation,
				cause,
				hint,
			)
		}
		return fmt.Errorf("Oracle Data Pump %s failed: %w", operation, cause)
	}
	if hint != "" {
		return fmt.Errorf(
			"Oracle Data Pump %s failed: %w; server log excerpt: %s; %s",
			operation,
			cause,
			detail,
			hint,
		)
	}
	return fmt.Errorf(
		"Oracle Data Pump %s failed: %w; server log excerpt: %s",
		operation,
		cause,
		detail,
	)
}

func oracleDataPumpFailureHint(cause error, detail string) string {
	message := strings.ToUpper(detail)
	if cause != nil {
		message += " " + strings.ToUpper(cause.Error())
	}
	if strings.Contains(message, "UNABLE TO LOAD XDB LIBRARY") {
		return "the Oracle installation cannot load XDB; Oracle Database Free must use the full, non-lite image for Data Pump"
	}
	if strings.Contains(message, "ORA-39029") ||
		strings.Contains(message, "ORA-31671") {
		return "the server-side Data Pump worker exited unexpectedly; verify XDB is installed, provide sufficient container shared memory, and inspect the Oracle worker trace"
	}
	return ""
}

func (o *Oracle) BackupDatabaseToWriter(
	ctx context.Context,
	writer io.Writer,
	request database.BackupRequest,
) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	if request.SchemaOnly || request.DataOnly {
		return fmt.Errorf(
			"Oracle Data Pump backups currently require structure and data together",
		)
	}
	directory, err := o.resolveBackupDirectory(ctx, request.Directory)
	if err != nil {
		return err
	}
	schema, err := o.resolveDataPumpSchema(ctx, request.Schema)
	if err != nil {
		return err
	}
	jobName, dumpFile, logFile, err := oracleDataPumpNames("EXP")
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(),
			oracleDataPumpCleanupTimeout,
		)
		defer cancel()
		o.removeServerFile(cleanupCtx, directory, dumpFile)
		o.removeServerFile(cleanupCtx, directory, logFile)
	}()

	if _, err := o.runDataPump(
		ctx,
		oracleDataPumpExportBlock,
		jobName,
		dumpFile,
		logFile,
		directory,
		schema,
	); err != nil {
		return o.dataPumpFailure(
			"export",
			jobName,
			directory,
			logFile,
			err,
		)
	}
	if err := o.copyServerFile(
		ctx,
		writer,
		directory,
		dumpFile,
		0,
		-1,
	); err != nil {
		return fmt.Errorf("download Oracle Data Pump backup: %w", err)
	}
	return nil
}

func (o *Oracle) RestoreDatabaseFromReader(
	ctx context.Context,
	reader io.Reader,
	request database.RestorePreviewRequest,
) error {
	if err := o.ensureConnected(); err != nil {
		return err
	}
	directory, err := o.resolveBackupDirectory(ctx, request.Directory)
	if err != nil {
		return err
	}
	schema, err := o.resolveDataPumpSchema(ctx, request.Schema)
	if err != nil {
		return err
	}
	jobName, dumpFile, logFile, err := oracleDataPumpNames("IMP")
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(),
			oracleDataPumpCleanupTimeout,
		)
		defer cancel()
		o.removeServerFile(cleanupCtx, directory, dumpFile)
		o.removeServerFile(cleanupCtx, directory, logFile)
	}()

	if err := o.writeServerFile(
		ctx,
		reader,
		directory,
		dumpFile,
	); err != nil {
		return err
	}
	if _, err := o.runDataPump(
		ctx,
		oracleDataPumpImportBlock,
		jobName,
		dumpFile,
		logFile,
		directory,
		schema,
	); err != nil {
		return o.dataPumpFailure(
			"import",
			jobName,
			directory,
			logFile,
			err,
		)
	}
	return nil
}

var _ database.StreamingBackupDriver = (*Oracle)(nil)
