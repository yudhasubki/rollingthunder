package db

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"rollingthunder/pkg/database"
)

type routingTestDriver struct {
	name string

	mu            sync.Mutex
	queries       []string
	closed        bool
	exportContent string
	exportCalls   int
	exportRequest database.TableExportRequest
	exportRows    int64
	exportErr     error
	exportStarted chan struct{}
	exportRelease chan struct{}
	exportOnce    sync.Once

	queryStarted chan struct{}
	queryRelease chan struct{}
	startOnce    sync.Once
	closeCalled  chan struct{}
	closeOnce    sync.Once

	transaction  database.Transaction
	beginErr     error
	beginContext context.Context

	changeRequest database.TableChangeSet
	changeResult  database.TableChangeResult
	changeErr     error
	structures    database.Structures
	queryRows     []map[string]interface{}

	objectFilter    database.ObjectFilter
	objectReference database.ObjectReference
	objects         []database.DatabaseObject
	objectDetail    database.ObjectDetail
	objectErr       error

	changePlan     database.ObjectChangePlan
	changePlanErr  error
	appliedPlan    database.ObjectChangePlan
	applyChangeErr error
}

func (d *routingTestDriver) Capabilities() database.Capabilities {
	return database.Capabilities{
		Engine:      "test",
		DisplayName: "Test",
		Dialect: database.Dialect{
			Name:                 "test",
			IdentifierOpen:       `"`,
			IdentifierClose:      `"`,
			PlaceholderStyle:     database.PlaceholderQuestion,
			PaginationStyle:      database.PaginationLimitOffset,
			SupportsNullOrdering: true,
		},
		Tables: true,
	}
}

func (d *routingTestDriver) QuoteIdentifier(identifier string) string {
	return `"` + identifier + `"`
}

func (d *routingTestDriver) Placeholder(int) string {
	return "?"
}

func (d *routingTestDriver) PaginationClause(limit, offset int) (string, error) {
	return "", nil
}

func (d *routingTestDriver) Connect(context.Context) error {
	return nil
}

func (d *routingTestDriver) Close() error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()

	if d.closeCalled != nil {
		d.closeOnce.Do(func() {
			close(d.closeCalled)
		})
	}

	return nil
}

func (d *routingTestDriver) CountCollectionData(database.Table) (int, error) {
	return 0, nil
}

func (d *routingTestDriver) GetCollectionData(database.Table) (database.Structures, []map[string]interface{}, error) {
	return nil, nil, nil
}

func (d *routingTestDriver) GetCollections(...string) ([]string, error) {
	return nil, nil
}

func (d *routingTestDriver) GetCollectionStructures(database.Table) (database.Structures, error) {
	return d.structures, nil
}

func (d *routingTestDriver) GetIndices(database.Table) (database.Indices, error) {
	return nil, nil
}

func (d *routingTestDriver) GetDatabaseInfo() (database.Info, error) {
	return database.Info{}, nil
}

func (d *routingTestDriver) InsertRow(database.Table, map[string]interface{}) error {
	return nil
}

func (d *routingTestDriver) UpdateRow(database.Table, map[string]interface{}, string) error {
	return nil
}

func (d *routingTestDriver) DeleteRow(database.Table, string, interface{}) error {
	return nil
}

func (d *routingTestDriver) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	d.mu.Lock()
	d.queries = append(d.queries, query)
	rows := d.queryRows
	d.mu.Unlock()

	if d.queryStarted != nil {
		d.startOnce.Do(func() {
			close(d.queryStarted)
		})
	}
	if d.queryRelease != nil {
		select {
		case <-d.queryRelease:
		case <-ctx.Done():
			return database.QueryResult{}, ctx.Err()
		}
	}

	if rows == nil {
		rows = []map[string]interface{}{{"source": d.name}}
	}
	return database.QueryResult{
		Rows:     rows,
		RowLimit: options.MaxRows,
	}, nil
}

func (d *routingTestDriver) BeginTransaction(
	ctx context.Context,
) (database.Transaction, error) {
	d.mu.Lock()
	d.beginContext = ctx
	d.mu.Unlock()
	if d.beginErr != nil {
		return nil, d.beginErr
	}
	if d.transaction == nil {
		return nil, errors.New("test transaction is not configured")
	}
	return d.transaction, nil
}

func (d *routingTestDriver) transactionContext() context.Context {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.beginContext
}

func (d *routingTestDriver) ApplyTableChanges(
	_ context.Context,
	changes database.TableChangeSet,
) (database.TableChangeResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.changeRequest = changes
	return d.changeResult, d.changeErr
}

func (d *routingTestDriver) ListObjects(
	_ context.Context,
	filter database.ObjectFilter,
) ([]database.DatabaseObject, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.objectFilter = filter
	return d.objects, d.objectErr
}

func (d *routingTestDriver) GetObjectDetail(
	_ context.Context,
	reference database.ObjectReference,
) (database.ObjectDetail, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.objectReference = reference
	return d.objectDetail, d.objectErr
}

func (d *routingTestDriver) BuildObjectChange(
	_ context.Context,
	_ database.ObjectChangeRequest,
) (database.ObjectChangePlan, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.changePlan, d.changePlanErr
}

func (d *routingTestDriver) ApplyObjectChange(
	_ context.Context,
	plan database.ObjectChangePlan,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.appliedPlan = plan
	return d.applyChangeErr
}

func (d *routingTestDriver) tableChangeRequest() database.TableChangeSet {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.changeRequest
}

func (d *routingTestDriver) ExportTable(
	ctx context.Context,
	request database.TableExportRequest,
	writer io.Writer,
) (database.ExportStats, error) {
	d.mu.Lock()
	d.exportCalls++
	d.exportRequest = request
	content := d.exportContent
	rows := d.exportRows
	exportErr := d.exportErr
	d.mu.Unlock()

	if content != "" {
		if _, err := io.WriteString(writer, content); err != nil {
			return database.ExportStats{}, err
		}
	}
	database.ReportExportProgress(ctx, rows)
	if d.exportStarted != nil {
		d.exportOnce.Do(func() {
			close(d.exportStarted)
		})
	}
	if d.exportRelease != nil {
		select {
		case <-d.exportRelease:
		case <-ctx.Done():
			return database.ExportStats{}, ctx.Err()
		}
	}
	if exportErr != nil {
		return database.ExportStats{}, exportErr
	}
	return database.ExportStats{Rows: rows}, nil
}

func (d *routingTestDriver) CreateTable(database.Table, []database.ColumnDefinition) error {
	return nil
}

func (d *routingTestDriver) DropTable(database.Table) error {
	return nil
}

func (d *routingTestDriver) TruncateTable(database.Table) error {
	return nil
}

func (d *routingTestDriver) GetTableDDL(database.Table) (string, error) {
	return "", nil
}

func (d *routingTestDriver) GetDataTypes() []database.DataType {
	return nil
}

func (d *routingTestDriver) queryCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.queries)
}

func (d *routingTestDriver) exportCallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.exportCalls
}

func newRoutingTestService(drivers map[string]*routingTestDriver, activeID string) *Service {
	service := NewService()
	service.Start(context.Background())
	connectedAt := time.Now()
	for id, driver := range drivers {
		service.connections[id] = &Connection{
			ID:          id,
			Name:        id,
			Driver:      driver,
			ConnectedAt: connectedAt,
		}
		connectedAt = connectedAt.Add(time.Second)
	}
	service.activeID = activeID

	return service
}

func TestExecuteQueryUsesOwningConnection(t *testing.T) {
	alpha := &routingTestDriver{name: "alpha"}
	bravo := &routingTestDriver{name: "bravo"}
	service := newRoutingTestService(
		map[string]*routingTestDriver{
			"alpha": alpha,
			"bravo": bravo,
		},
		"bravo",
	)

	response := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: "alpha",
		Query:        "select current_database()",
	})

	if len(response.Errors) != 0 {
		t.Fatalf("ExecuteQuery returned errors: %+v", response.Errors)
	}
	if len(response.Data.Rows) != 1 || response.Data.Rows[0]["source"] != "alpha" {
		t.Fatalf("ExecuteQuery returned data from the wrong connection: %+v", response.Data)
	}
	if response.Data.RowLimit != database.DefaultQueryResultLimit {
		t.Fatalf(
			"ExecuteQuery row limit = %d, want %d",
			response.Data.RowLimit,
			database.DefaultQueryResultLimit,
		)
	}
	if alpha.queryCount() != 1 {
		t.Fatalf("alpha query count = %d, want 1", alpha.queryCount())
	}
	if bravo.queryCount() != 0 {
		t.Fatalf("bravo query count = %d, want 0", bravo.queryCount())
	}

	if switched := service.SwitchConnection("alpha"); len(switched.Errors) != 0 || !switched.Data {
		t.Fatalf("SwitchConnection failed: %+v", switched)
	}

	response = service.ExecuteQuery(database.QueryRequest{
		ConnectionID: "bravo",
		Query:        "select current_database()",
	})
	if len(response.Errors) != 0 {
		t.Fatalf("ExecuteQuery returned errors after switching: %+v", response.Errors)
	}
	if len(response.Data.Rows) != 1 || response.Data.Rows[0]["source"] != "bravo" {
		t.Fatalf("ExecuteQuery followed the global active connection: %+v", response.Data)
	}
}

func TestConnectionSensitiveOperationsRejectMissingConnection(t *testing.T) {
	service := NewService()

	for name, response := range map[string]struct {
		errors int
	}{
		"empty": {errors: len(service.ExecuteQuery(database.QueryRequest{
			Query: "select 1",
		}).Errors)},
		"unknown": {errors: len(service.ExecuteQuery(database.QueryRequest{
			ConnectionID: "missing",
			Query:        "select 1",
		}).Errors)},
	} {
		if response.errors != 1 {
			t.Errorf("%s connection returned %d errors, want 1", name, response.errors)
		}
	}
}

func TestDisconnectWaitsForInFlightOperation(t *testing.T) {
	driver := &routingTestDriver{
		name:         "alpha",
		queryStarted: make(chan struct{}),
		queryRelease: make(chan struct{}),
		closeCalled:  make(chan struct{}),
	}
	service := newRoutingTestService(
		map[string]*routingTestDriver{"alpha": driver},
		"alpha",
	)

	queryDone := make(chan struct{})
	go func() {
		service.ExecuteQuery(database.QueryRequest{
			ConnectionID: "alpha",
			Query:        "select pg_sleep(1)",
		})
		close(queryDone)
	}()

	select {
	case <-driver.queryStarted:
	case <-time.After(time.Second):
		t.Fatal("query did not start")
	}

	disconnectDone := make(chan struct{})
	go func() {
		service.DisconnectConnection("alpha")
		close(disconnectDone)
	}()

	select {
	case <-driver.closeCalled:
		t.Fatal("driver closed while a query was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(driver.queryRelease)

	select {
	case <-queryDone:
	case <-time.After(time.Second):
		t.Fatal("query did not finish after it was released")
	}
	select {
	case <-disconnectDone:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not finish after the query completed")
	}
	select {
	case <-driver.closeCalled:
	default:
		t.Fatal("driver was not closed after in-flight work completed")
	}

	response := service.ExecuteQuery(database.QueryRequest{
		ConnectionID: "alpha",
		Query:        "select 1",
	})
	if len(response.Errors) != 1 {
		t.Fatalf("disconnected connection returned %d errors, want 1", len(response.Errors))
	}
}
