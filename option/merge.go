package option

type Option = map[string]any

func Merge(defaultOption Option, targetOption Option) {
	for key, _ := range defaultOption {
		_, ok := targetOption[key]
		if !ok {
			targetOption[key] = defaultOption[key]
		}
	}
}
