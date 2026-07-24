package db

import (
	"context"
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
	return nil, nil
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
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	d.mu.Lock()
	d.queries = append(d.queries, query)
	d.mu.Unlock()

	if d.queryStarted != nil {
		d.startOnce.Do(func() {
			close(d.queryStarted)
		})
	}
	if d.queryRelease != nil {
		<-d.queryRelease
	}

	return database.QueryResult{
		Rows:     []map[string]interface{}{{"source": d.name}},
		RowLimit: options.MaxRows,
	}, nil
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

	response := service.ExecuteQuery("alpha", "select current_database()")

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

	response = service.ExecuteQuery("bravo", "select current_database()")
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
		"empty":   {errors: len(service.ExecuteQuery("", "select 1").Errors)},
		"unknown": {errors: len(service.ExecuteQuery("missing", "select 1").Errors)},
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
		service.ExecuteQuery("alpha", "select pg_sleep(1)")
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

	response := service.ExecuteQuery("alpha", "select 1")
	if len(response.Errors) != 1 {
		t.Fatalf("disconnected connection returned %d errors, want 1", len(response.Errors))
	}
}
