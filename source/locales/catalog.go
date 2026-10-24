package locales

func Text(key string) string { return Catalog.Format("zh-Hans", key, nil) }
