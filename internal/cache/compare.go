package cache

func ComparePorts(cachePorts, backendPorts []Port) (result []Port) {
	backendPorts = removeDuplicates(backendPorts)
	cachePorts = removeDuplicates(cachePorts)
	for k, v := range backendPorts {
		cachePorts, backendPorts[k] = Compare(cachePorts, v)
	}
	return backendPorts
}

func Compare(a []Port, e Port) ([]Port, Port) {
	if contains(a, e) {
		e.Port += 1
		return Compare(a, e)
	}
	a = append(a, e)
	return a, e
}

func removeDuplicates(s []Port) []Port {
	var x []Port
	for _, e := range s {
		if !contains(x, e) {
			x = append(x, e)
		}
	}
	return x
}

func contains(s []Port, e Port) bool {
	for _, a := range s {
		if a.Port == e.Port {
			return true
		}
	}
	return false
}
