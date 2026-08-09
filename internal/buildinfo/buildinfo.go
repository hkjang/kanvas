package buildinfo

var (
	Version = "0.4.0-dev"
	Commit  = "unknown"
	BuiltAt = "unknown"
)

type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
}

func Current() Info {
	return Info{Name: "Kanvas", Version: Version, Commit: Commit, BuiltAt: BuiltAt}
}
