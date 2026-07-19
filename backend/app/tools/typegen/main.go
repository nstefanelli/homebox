package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const generatedTypesPath = "frontend/lib/api/types/data-contracts.ts"

type ReReplace struct {
	Regex *regexp.Regexp
	Text  string
}

func NewReReplace(regex string, replace string) ReReplace {
	return ReReplace{
		Regex: regexp.MustCompile(regex),
		Text:  replace,
	}
}

func NewReDate(dateStr string) ReReplace {
	return ReReplace{
		Regex: regexp.MustCompile(fmt.Sprintf(`%s: string`, dateStr)),
		Text:  fmt.Sprintf(`%s: Date | string`, dateStr),
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Please provide a file path as an argument")
		os.Exit(1)
	}

	if filepath.Clean(os.Args[1]) != filepath.Clean(generatedTypesPath) {
		fmt.Printf("Refusing to process unexpected path %q\n", os.Args[1])
		os.Exit(1)
	}
	fmt.Printf("Processing %s\n", generatedTypesPath)

	text := "/* post-processed by ./scripts/process-types.go */\n"
	data, err := os.ReadFile(generatedTypesPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	text += string(data)

	replaces := [...]ReReplace{
		NewReReplace(` Repo`, " "),
		NewReReplace(` PaginationResultRepo`, " PaginationResult"),
		NewReReplace(` Services`, " "),
		NewReReplace(` V1`, " "),
		NewReReplace(`\?:`, ":"),
		NewReReplace(`(\w+):\s(.*null.*)`, "$1?: $2"), // make null union types optional
		NewReReplace(`(?m)^(\s*)timeValue: string;$`, "${1}timeValue: string | null;"),
		NewReDate("createdAt"),
		NewReDate("updatedAt"),
		NewReDate("soldDate"),
		NewReDate("purchaseDate"),
		NewReDate("warrantyExpires"),
		NewReDate("expiresAt"),
		NewReDate("date"),
		NewReDate("completedDate"),
		NewReDate("scheduledDate"),
	}

	for _, replace := range replaces {
		fmt.Printf("Replacing '%v' -> '%s'\n", replace.Regex, replace.Text)
		text = replace.Regex.ReplaceAllString(text, replace.Text)
	}

	// #nosec G306,G703 -- the generated source is intentionally
	// repository-readable and generatedTypesPath is a fixed compile-time path.
	err = os.WriteFile(generatedTypesPath, []byte(text), 0o644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	os.Exit(0)
}
