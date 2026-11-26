package catalogsource

// TargetSetsDir — каталог наборов оценки «по цели» (mapping.target_sets, путь от base_dir); "" — наборов нет.
func (g *Generic) TargetSetsDir() string {
	if g.mapping.TargetSets == "" {
		return ""
	}
	return g.mapping.path(g.mapping.TargetSets)
}
