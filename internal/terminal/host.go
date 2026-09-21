package terminal

func unicodeHostArguments(executable, directory string) []string {
	return []string{"-w", "new", "new-tab", "--profile", "XChat", "--startingDirectory", directory, executable}
}
