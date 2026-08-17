package gh

import "encoding/json"

// narrowSearch turns one recorded or live `gh api graphql` response into pull requests. A response
// that is not shaped as expected yields none rather than an error, so an upstream schema change
// costs signals, not the command.
func narrowSearch(raw []byte) []Pr {
	document, ok := decodeRecord(raw)
	if !ok {
		return []Pr{}
	}
	return narrowSearchDocument(document)
}

// narrowSearchDocument narrows an already-decoded response, which is how a recorded fixture reaches
// exactly the path a live response takes.
func narrowSearchDocument(document any) []Pr {
	response, ok := document.(map[string]any)
	if !ok {
		return []Pr{}
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		return []Pr{}
	}
	search, ok := data["search"].(map[string]any)
	if !ok {
		return []Pr{}
	}
	return narrowPrs(search["nodes"])
}

// narrowPrs narrows either gh's output or our own cache; anything that is not a pull request is
// dropped rather than trusted.
func narrowPrs(value any) []Pr {
	nodes, ok := value.([]any)
	if !ok {
		return []Pr{}
	}

	prs := make([]Pr, 0, len(nodes))
	for _, node := range nodes {
		if pr, ok := narrowPr(node); ok {
			prs = append(prs, pr)
		}
	}
	return prs
}

func narrowPr(value any) (Pr, bool) {
	node, ok := value.(map[string]any)
	if !ok {
		return Pr{}, false
	}

	number, ok := finiteNumber(node["number"])
	if !ok {
		return Pr{}, false
	}
	title, ok := node["title"].(string)
	if !ok {
		return Pr{}, false
	}
	repository, ok := repositoryName(node["repository"])
	if !ok {
		return Pr{}, false
	}

	return Pr{
		Number:     number,
		Repository: repository,
		Title:      title,
		Author:     authorLogin(node["author"]),
		IsDraft:    node["isDraft"] == true,
		State:      stringOr(node["state"], "open"),
		CreatedAt:  stringOr(node["createdAt"], ""),
		Url:        stringOr(node["url"], ""),
		Branch:     branchName(node),
	}, true
}

// branchName reads the head branch: GraphQL names it headRefName, our own cache stores it flat as
// branch.
func branchName(node map[string]any) string {
	if head, ok := node["headRefName"].(string); ok {
		return head
	}
	return stringOr(node["branch"], "")
}

// repositoryName reads owner/repo: gh nests the repo as an object, our own cache stores it flat.
func repositoryName(value any) (string, bool) {
	if name, ok := value.(string); ok {
		return name, name != ""
	}
	node, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	withOwner, ok := node["nameWithOwner"].(string)
	return withOwner, ok && withOwner != ""
}

func authorLogin(value any) string {
	if login, ok := value.(string); ok {
		return login
	}
	if node, ok := value.(map[string]any); ok {
		if login, ok := node["login"].(string); ok {
			return login
		}
	}
	return ""
}

// decodeRecord decodes raw into a JSON object, reporting false for anything else — an array, a
// scalar, or bytes that are not JSON at all.
func decodeRecord(raw []byte) (map[string]any, bool) {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, false
	}
	record, ok := parsed.(map[string]any)
	return record, ok
}

// finiteNumber narrows a JSON number to an int, rejecting the infinities and NaN a JSON document
// cannot carry but a hand-edited cache might be read as.
func finiteNumber(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok || number != number || number > 1e15 || number < -1e15 {
		return 0, false
	}
	return int(number), true
}

func isNumber(value any, want int) bool {
	number, ok := finiteNumber(value)
	return ok && number == want
}

func stringOr(value any, fallback string) string {
	if text, ok := value.(string); ok {
		return text
	}
	return fallback
}
