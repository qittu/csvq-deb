package query

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mithrandie/csvq/lib/cmd"
	"github.com/mithrandie/csvq/lib/parser"
	"github.com/mithrandie/csvq/lib/value"

	"github.com/mithrandie/ternary"
)

const LimitToUseFieldIndexSliceChache = 8

var blockScopePool = sync.Pool{
	New: func() interface{} {
		return NewBlockScope()
	},
}

func GetBlockScope() BlockScope {
	scope := blockScopePool.Get().(BlockScope)
	return scope
}

func PutBlockScope(scope BlockScope) {
	scope.Clear()
	blockScopePool.Put(scope)
}

var nodeScopePool = sync.Pool{
	New: func() interface{} {
		return NewNodeScope()
	},
}

func GetNodeScope() NodeScope {
	scope := nodeScopePool.Get().(NodeScope)
	return scope
}

func PutNodeScope(scope NodeScope) {
	scope.Clear()
	nodeScopePool.Put(scope)
}

type BlockScope struct {
	variables       VariableMap
	temporaryTables ViewMap
	cursors         CursorMap
	functions       UserDefinedFunctionMap
}

func NewBlockScope() BlockScope {
	return BlockScope{
		variables:       NewVariableMap(),
		temporaryTables: NewViewMap(),
		cursors:         NewCursorMap(),
		functions:       NewUserDefinedFunctionMap(),
	}
}

func (scope BlockScope) Clear() {
	scope.variables.Clear()
	scope.temporaryTables.Clear()
	scope.cursors.Clear()
	scope.functions.Clear()
}

type NodeScope struct {
	inlineTables InlineTableMap
	aliases      AliasMap
}

func NewNodeScope() NodeScope {
	return NodeScope{
		inlineTables: make(InlineTableMap),
		aliases:      make(AliasMap),
	}
}

func (scope NodeScope) Clear() {
	scope.inlineTables.Clear()
	scope.aliases.Clear()
}

type ReferenceRecord struct {
	view        *View
	recordIndex int

	cache *FieldIndexCache
}

func NewReferenceRecord(view *View, recordIdx int, cacheLen int) ReferenceRecord {
	return ReferenceRecord{
		view:        view,
		recordIndex: recordIdx,
		cache:       NewFieldIndexCache(cacheLen, LimitToUseFieldIndexSliceChache),
	}
}

func (r *ReferenceRecord) IsInRange() bool {
	return -1 < r.recordIndex && r.recordIndex < r.view.RecordLen()
}

type FieldIndexCache struct {
	limitToUseSlice int
	m               map[parser.QueryExpression]int
	exprs           []parser.QueryExpression
	indices         []int
}

func NewFieldIndexCache(initCap int, limitToUseSlice int) *FieldIndexCache {
	return &FieldIndexCache{
		limitToUseSlice: limitToUseSlice,
		m:               nil,
		exprs:           make([]parser.QueryExpression, 0, initCap),
		indices:         make([]int, 0, initCap),
	}
}

func (c *FieldIndexCache) Get(expr parser.QueryExpression) (int, bool) {
	if c.m != nil {
		idx, ok := c.m[expr]
		return idx, ok
	}

	for i := range c.exprs {
		if expr == c.exprs[i] {
			return c.indices[i], true
		}
	}
	return -1, false
}

func (c *FieldIndexCache) Add(expr parser.QueryExpression, idx int) {
	if c.m == nil && c.limitToUseSlice <= len(c.exprs) {
		c.m = make(map[parser.QueryExpression]int, c.limitToUseSlice*2)
		for i := range c.exprs {
			c.m[c.exprs[i]] = c.indices[i]
		}
		c.exprs = nil
		c.indices = nil
	}

	if c.m == nil {
		c.exprs = append(c.exprs, expr)
		c.indices = append(c.indices, idx)
	} else {
		c.m[expr] = idx
	}
}

type ReferenceScope struct {
	Tx *Transaction

	blocks []BlockScope
	nodes  []NodeScope

	cachedFilePath map[string]string
	now            time.Time

	Records []ReferenceRecord

	RecursiveTable   *parser.InlineTable
	RecursiveTmpView *View
	RecursiveCount   *int64
}

func NewReferenceScope(tx *Transaction) *ReferenceScope {
	return NewReferenceScopeWithBlock(tx, GetBlockScope())
}

func NewReferenceScopeWithBlock(tx *Transaction, scope BlockScope) *ReferenceScope {
	return &ReferenceScope{
		Tx:     tx,
		blocks: []BlockScope{scope},
		nodes:  nil,
	}
}

func (rs *ReferenceScope) CreateScopeForRecordEvaluation(view *View, recordIndex int) *ReferenceScope {
	records := make([]ReferenceRecord, len(rs.Records)+1)
	records[0] = NewReferenceRecord(view, recordIndex, view.FieldLen())
	for i := range rs.Records {
		records[i+1] = rs.Records[i]
	}
	return rs.createScope(records)
}

func (rs *ReferenceScope) CreateScopeForSequentialEvaluation(view *View) *ReferenceScope {
	return rs.CreateScopeForRecordEvaluation(view, -1)
}

func (rs *ReferenceScope) CreateScopeForAnalytics() *ReferenceScope {
	records := make([]ReferenceRecord, len(rs.Records))
	records[0] = NewReferenceRecord(rs.Records[0].view, -1, rs.Records[0].view.FieldLen())
	for i := 1; i < len(rs.Records); i++ {
		records[i] = rs.Records[i]
	}
	return rs.createScope(records)
}

func (rs *ReferenceScope) createScope(referenceRecords []ReferenceRecord) *ReferenceScope {
	return &ReferenceScope{
		Tx:               rs.Tx,
		blocks:           rs.blocks,
		nodes:            rs.nodes,
		cachedFilePath:   rs.cachedFilePath,
		now:              rs.now,
		Records:          referenceRecords,
		RecursiveTable:   rs.RecursiveTable,
		RecursiveTmpView: rs.RecursiveTmpView,
		RecursiveCount:   rs.RecursiveCount,
	}
}

func (rs *ReferenceScope) CreateChild() *ReferenceScope {
	blocks := make([]BlockScope, len(rs.blocks)+1)
	blocks[0] = GetBlockScope()
	for i := range rs.blocks {
		blocks[i+1] = rs.blocks[i]
	}

	return &ReferenceScope{
		Tx:               rs.Tx,
		blocks:           blocks,
		nodes:            nil,
		cachedFilePath:   rs.cachedFilePath,
		now:              rs.now,
		RecursiveTable:   rs.RecursiveTable,
		RecursiveTmpView: rs.RecursiveTmpView,
		RecursiveCount:   rs.RecursiveCount,
	}
}

func (rs *ReferenceScope) CreateNode() *ReferenceScope {
	nodes := make([]NodeScope, len(rs.nodes)+1)
	nodes[0] = GetNodeScope()
	for i := range rs.nodes {
		nodes[i+1] = rs.nodes[i]
	}

	node := &ReferenceScope{
		Tx:               rs.Tx,
		blocks:           rs.blocks,
		nodes:            nodes,
		cachedFilePath:   rs.cachedFilePath,
		now:              rs.now,
		Records:          rs.Records,
		RecursiveTable:   rs.RecursiveTable,
		RecursiveTmpView: rs.RecursiveTmpView,
		RecursiveCount:   rs.RecursiveCount,
	}

	if node.cachedFilePath == nil {
		node.cachedFilePath = make(map[string]string)
	}
	if node.now.IsZero() {
		node.now = cmd.Now()
	}

	return node
}

func (rs *ReferenceScope) Global() BlockScope {
	return rs.blocks[len(rs.blocks)-1]
}

func (rs *ReferenceScope) CurrentBlock() BlockScope {
	return rs.blocks[0]
}

func (rs *ReferenceScope) ClearCurrentBlock() {
	rs.CurrentBlock().Clear()
}

func (rs *ReferenceScope) CloseCurrentBlock() {
	PutBlockScope(rs.CurrentBlock())
}

func (rs *ReferenceScope) CloseCurrentNode() {
	PutNodeScope(rs.nodes[0])
}

func (rs *ReferenceScope) NextRecord() bool {
	rs.Records[0].recordIndex++

	if rs.Records[0].view.Len() <= rs.Records[0].recordIndex {
		return false
	}
	return true
}

func (rs *ReferenceScope) StoreFilePath(identifier string, fpath string) {
	if rs.cachedFilePath != nil {
		rs.cachedFilePath[identifier] = fpath
	}
}

func (rs *ReferenceScope) LoadFilePath(identifier string) (string, bool) {
	if rs.cachedFilePath != nil {
		if p, ok := rs.cachedFilePath[identifier]; ok {
			return p, true
		}
	}
	return "", false
}

func (rs *ReferenceScope) Now() time.Time {
	if rs.now.IsZero() {
		return cmd.Now()
	}
	return rs.now
}

func (rs *ReferenceScope) DeclareVariable(ctx context.Context, expr parser.VariableDeclaration) error {
	return rs.blocks[0].variables.Declare(ctx, rs, expr)
}

func (rs *ReferenceScope) DeclareVariableDirectly(variable parser.Variable, val value.Primary) error {
	return rs.blocks[0].variables.Add(variable, val)
}

func (rs *ReferenceScope) GetVariable(expr parser.Variable) (val value.Primary, err error) {
	for i := range rs.blocks {
		if v, ok := rs.blocks[i].variables.Get(expr); ok {
			return v, nil
		}
	}
	return nil, NewUndeclaredVariableError(expr)
}

func (rs *ReferenceScope) SubstituteVariable(ctx context.Context, expr parser.VariableSubstitution) (val value.Primary, err error) {
	val, err = Evaluate(ctx, rs, expr.Value)
	if err != nil {
		return
	}

	for i := range rs.blocks {
		if rs.blocks[i].variables.Set(expr.Variable, val) {
			return
		}
	}
	err = NewUndeclaredVariableError(expr.Variable)
	return
}

func (rs *ReferenceScope) SubstituteVariableDirectly(variable parser.Variable, val value.Primary) (value.Primary, error) {
	for i := range rs.blocks {
		if rs.blocks[i].variables.Set(variable, val) {
			return val, nil
		}
	}
	return nil, NewUndeclaredVariableError(variable)
}

func (rs *ReferenceScope) DisposeVariable(expr parser.Variable) error {
	for i := range rs.blocks {
		if rs.blocks[i].variables.Dispose(expr) {
			return nil
		}
	}
	return NewUndeclaredVariableError(expr)
}

func (rs *ReferenceScope) AllVariables() VariableMap {
	all := NewVariableMap()
	for i := range rs.blocks {
		rs.blocks[i].variables.Range(func(key, val interface{}) bool {
			if !all.Exists(key.(string)) {
				all.Store(key.(string), val.(value.Primary))
			}
			return true
		})
	}
	return all
}

func (rs *ReferenceScope) TemporaryTableExists(name string) bool {
	for i := range rs.blocks {
		if rs.blocks[i].temporaryTables.Exists(name) {
			return true
		}
	}
	return false
}

func (rs *ReferenceScope) GetTemporaryTable(name parser.Identifier) (*View, error) {
	for i := range rs.blocks {
		if view, err := rs.blocks[i].temporaryTables.Get(name); err == nil {
			return view, nil
		}
	}
	return nil, NewUndeclaredTemporaryTableError(name)
}

func (rs *ReferenceScope) GetTemporaryTableWithInternalId(ctx context.Context, name parser.Identifier, flags *cmd.Flags) (view *View, err error) {
	for i := range rs.blocks {
		if view, err = rs.blocks[i].temporaryTables.GetWithInternalId(ctx, name, flags); err == nil {
			return
		} else if err != errTableNotLoaded {
			return nil, err
		}
	}
	return nil, NewUndeclaredTemporaryTableError(name)
}

func (rs *ReferenceScope) SetTemporaryTable(view *View) {
	rs.blocks[0].temporaryTables.Set(view)
}

func (rs *ReferenceScope) ReplaceTemporaryTable(view *View) {
	for i := range rs.blocks {
		if rs.blocks[i].temporaryTables.Exists(view.FileInfo.Path) {
			rs.blocks[i].temporaryTables.Set(view)
			return
		}
	}
}

func (rs *ReferenceScope) DisposeTemporaryTable(name parser.QueryExpression) error {
	for i := range rs.blocks {
		if rs.blocks[i].temporaryTables.DisposeTemporaryTable(name) {
			return nil
		}
	}
	return NewUndeclaredTemporaryTableError(name)
}

func (rs *ReferenceScope) StoreTemporaryTable(session *Session, uncomittedViews map[string]*FileInfo) []string {
	msglist := make([]string, 0, len(uncomittedViews))
	for i := range rs.blocks {
		rs.blocks[i].temporaryTables.Range(func(key, value interface{}) bool {
			if _, ok := uncomittedViews[key.(string)]; ok {
				view := value.(*View)

				if view.FileInfo.IsStdin() {
					session.updateStdinView(view.Copy())
				} else {
					view.CreateRestorePoint()
				}
				msglist = append(msglist, fmt.Sprintf("Commit: restore point of view %q is created.", view.FileInfo.Path))
			}
			return true
		})
	}
	return msglist
}

func (rs *ReferenceScope) RestoreTemporaryTable(uncomittedViews map[string]*FileInfo) []string {
	msglist := make([]string, 0, len(uncomittedViews))
	for i := range rs.blocks {
		rs.blocks[i].temporaryTables.Range(func(key, value interface{}) bool {
			if _, ok := uncomittedViews[key.(string)]; ok {
				view := value.(*View)

				if view.FileInfo.IsStdin() {
					rs.blocks[i].temporaryTables.Delete(view.FileInfo.Path)
				} else {
					view.Restore()
				}
				msglist = append(msglist, fmt.Sprintf("Rollback: view %q is restored.", view.FileInfo.Path))
			}
			return true
		})
	}
	return msglist
}

func (rs *ReferenceScope) AllTemporaryTables() ViewMap {
	all := NewViewMap()

	for i := range rs.blocks {
		rs.blocks[i].temporaryTables.Range(func(key, value interface{}) bool {
			if !value.(*View).FileInfo.IsFile() {
				k := key.(string)
				if !all.Exists(k) {
					all.Store(k, value.(*View))
				}
			}
			return true
		})
	}
	return all
}

func (rs *ReferenceScope) DeclareCursor(expr parser.CursorDeclaration) error {
	return rs.blocks[0].cursors.Declare(expr)
}

func (rs *ReferenceScope) AddPseudoCursor(name parser.Identifier, values []value.Primary) error {
	return rs.blocks[0].cursors.AddPseudoCursor(name, values)
}

func (rs *ReferenceScope) DisposeCursor(name parser.Identifier) error {
	for i := range rs.blocks {
		err := rs.blocks[i].cursors.Dispose(name)
		if err == nil {
			return nil
		}
		if err == errPseudoCursor {
			return NewPseudoCursorError(name)
		}
	}
	return NewUndeclaredCursorError(name)
}

func (rs *ReferenceScope) OpenCursor(ctx context.Context, name parser.Identifier, values []parser.ReplaceValue) error {
	var err error
	for i := range rs.blocks {
		err = rs.blocks[i].cursors.Open(ctx, rs, name, values)
		if err == nil {
			return nil
		}
		if err != errUndeclaredCursor {
			return err
		}
	}
	return NewUndeclaredCursorError(name)
}

func (rs *ReferenceScope) CloseCursor(name parser.Identifier) error {
	for i := range rs.blocks {
		err := rs.blocks[i].cursors.Close(name)
		if err == nil {
			return nil
		}
		if err != errUndeclaredCursor {
			return err
		}
	}
	return NewUndeclaredCursorError(name)
}

func (rs *ReferenceScope) FetchCursor(name parser.Identifier, position int, number int) ([]value.Primary, error) {
	var values []value.Primary
	var err error

	for i := range rs.blocks {
		values, err = rs.blocks[i].cursors.Fetch(name, position, number)
		if err == nil {
			return values, nil
		}
		if err != errUndeclaredCursor {
			return nil, err
		}
	}
	return nil, NewUndeclaredCursorError(name)
}

func (rs *ReferenceScope) CursorIsOpen(name parser.Identifier) (ternary.Value, error) {
	for i := range rs.blocks {
		if ok, err := rs.blocks[i].cursors.IsOpen(name); err == nil {
			return ok, nil
		}
	}
	return ternary.FALSE, NewUndeclaredCursorError(name)
}

func (rs *ReferenceScope) CursorIsInRange(name parser.Identifier) (ternary.Value, error) {
	var result ternary.Value
	var err error

	for i := range rs.blocks {
		result, err = rs.blocks[i].cursors.IsInRange(name)
		if err == nil {
			return result, nil
		}
		if err != errUndeclaredCursor {
			return result, err
		}
	}
	return ternary.FALSE, NewUndeclaredCursorError(name)
}

func (rs *ReferenceScope) CursorCount(name parser.Identifier) (int, error) {
	var count int
	var err error

	for i := range rs.blocks {
		count, err = rs.blocks[i].cursors.Count(name)
		if err == nil {
			return count, nil
		}
		if err != errUndeclaredCursor {
			return 0, err
		}
	}
	return 0, NewUndeclaredCursorError(name)
}

func (rs *ReferenceScope) AllCursors() CursorMap {
	all := NewCursorMap()
	for i := range rs.blocks {
		rs.blocks[i].cursors.Range(func(key, val interface{}) bool {
			cur := val.(*Cursor)
			if !cur.isPseudo {
				if !all.Exists(key.(string)) {
					all.Store(key.(string), cur)
				}
			}
			return true
		})
	}
	return all
}

func (rs *ReferenceScope) DeclareFunction(expr parser.FunctionDeclaration) error {
	return rs.blocks[0].functions.Declare(expr)
}

func (rs *ReferenceScope) DeclareAggregateFunction(expr parser.AggregateDeclaration) error {
	return rs.blocks[0].functions.DeclareAggregate(expr)
}

func (rs *ReferenceScope) GetFunction(expr parser.QueryExpression, name string) (*UserDefinedFunction, error) {
	for i := range rs.blocks {
		if fn, ok := rs.blocks[i].functions.Get(expr, name); ok {
			return fn, nil
		}
	}
	return nil, NewFunctionNotExistError(expr, name)
}

func (rs *ReferenceScope) DisposeFunction(name parser.Identifier) error {
	for i := range rs.blocks {
		if rs.blocks[i].functions.Dispose(name) {
			return nil
		}
	}
	return NewFunctionNotExistError(name, name.Literal)
}

func (rs *ReferenceScope) AllFunctions() (UserDefinedFunctionMap, UserDefinedFunctionMap) {
	scalarAll := NewUserDefinedFunctionMap()
	aggregateAll := NewUserDefinedFunctionMap()

	for i := range rs.blocks {
		rs.blocks[i].functions.Range(func(key, val interface{}) bool {
			fn := val.(*UserDefinedFunction)
			if fn.IsAggregate {
				if !aggregateAll.Exists(key.(string)) {
					aggregateAll.Store(key.(string), fn)
				}
			} else {
				if !scalarAll.Exists(key.(string)) {
					scalarAll.Store(key.(string), fn)
				}
			}
			return true
		})
	}

	return scalarAll, aggregateAll
}

func (rs *ReferenceScope) SetInlineTable(ctx context.Context, inlineTable parser.InlineTable) error {
	return rs.nodes[0].inlineTables.Set(ctx, rs, inlineTable)
}

func (rs *ReferenceScope) GetInlineTable(name parser.Identifier) (*View, error) {
	for i := range rs.nodes {
		if view, err := rs.nodes[i].inlineTables.Get(name); err == nil {
			return view, nil
		}
	}
	return nil, NewUndefinedInLineTableError(name)
}

func (rs *ReferenceScope) StoreInlineTable(name parser.Identifier, view *View) error {
	return rs.nodes[0].inlineTables.Store(name, view)
}

func (rs *ReferenceScope) InlineTableExists(name parser.Identifier) bool {
	for i := range rs.nodes {
		if rs.nodes[i].inlineTables.Exists(name) {
			return true
		}
	}
	return false
}

func (rs *ReferenceScope) LoadInlineTable(ctx context.Context, clause parser.WithClause) error {
	for _, v := range clause.InlineTables {
		inlineTable := v.(parser.InlineTable)
		err := rs.SetInlineTable(ctx, inlineTable)
		if err != nil {
			return err
		}
	}

	return nil
}

func (rs *ReferenceScope) AddAlias(alias parser.Identifier, path string) error {
	return rs.nodes[0].aliases.Add(alias, path)
}

func (rs *ReferenceScope) GetAlias(alias parser.Identifier) (path string, err error) {
	for i := range rs.nodes {
		if path, err = rs.nodes[i].aliases.Get(alias); err == nil {
			return
		}
	}
	err = NewTableNotLoadedError(alias)
	return
}
