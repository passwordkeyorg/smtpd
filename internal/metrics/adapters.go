package metrics

// Adapters keep internal packages decoupled from prometheus types.

type IndexerAdapter struct{ M IndexerMetrics }

type UploaderAdapter struct{ M UploaderMetrics }

func (a IndexerAdapter) IncRun(scanned, indexed int) {
	a.M.RunsTotal.Inc()
	a.M.ScannedTotal.Add(float64(scanned))
	a.M.IndexedTotal.Add(float64(indexed))
}
func (a IndexerAdapter) IncError()                { a.M.ErrorsTotal.Inc() }
func (a IndexerAdapter) SetLastRunUnix(t float64) { a.M.LastRunUnix.Set(t) }

func (a UploaderAdapter) IncRun(scanned, uploaded int) {
	a.M.RunsTotal.Inc()
	a.M.ScannedTotal.Add(float64(scanned))
	a.M.UploadedTotal.Add(float64(uploaded))
}
func (a UploaderAdapter) IncError()                { a.M.ErrorsTotal.Inc() }
func (a UploaderAdapter) SetLastRunUnix(t float64) { a.M.LastRunUnix.Set(t) }
