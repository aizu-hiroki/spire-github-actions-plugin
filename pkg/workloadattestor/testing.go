package workloadattestor

// SetProcEnvReaderForTest replaces the /proc reader with a test function.
// Must not be called in production code.
func (p *Plugin) SetProcEnvReaderForTest(fn func(pid int32) (map[string]string, error)) {
	p.mu.Lock()
	p.procEnvReader = fn
	p.mu.Unlock()
}
