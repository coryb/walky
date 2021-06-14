package walky_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/coryb/walky"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func StderrTracer(cur, par *yaml.Node, pos, depth int, ws walky.WalkStatus, err error) {
	indent := strings.Repeat("===", depth)
	fmt.Fprintf(os.Stderr, "%s> %d %s %q [%s, %v]\n", indent, pos, cur.Tag, cur.Value, ws, err)
}

func Here(doc string) string {
	// tabs are forbidden in yaml, so filter them out now
	return heredoc.Doc(regexp.MustCompilePOSIX("^\t+").ReplaceAllStringFunc(doc, func(in string) string {
		return strings.Repeat("    ", len(in))
	}))
}

func HereBytes(doc string) []byte {
	return []byte(Here(doc))
}
func TestWalk(t *testing.T) {
	doc := HereBytes(`
	a: [1,2,3]
	b: 
		c: [4,5]
		d: 
			e: [6, 7]
		f: [8,9]
	g:
		h: [10, 11]
	h: 
	 - 12
	 - 13
	 - i: [14, 15]
	`)
	var root yaml.Node
	err := yaml.Unmarshal(doc, &root)
	require.NoError(t, err)

	depthFirst := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15"}
	collected := []string{}
	err = walky.Walk(&root, walky.ScalarValuesWalker(func(n *yaml.Node) error {
		collected = append(collected, n.Value)
		return nil
	}))
	require.NoError(t, err)
	require.Equal(t, depthFirst, collected)

	breadthFirst := []string{"1", "2", "3", "12", "13", "4", "5", "8", "9", "10", "11", "6", "7", "14", "15"}
	collected = []string{}
	err = walky.Walk(&root, walky.ScalarValuesWalker(func(n *yaml.Node) error {
		collected = append(collected, n.Value)
		return nil
	}), walky.WithBreadthFirst())
	require.NoError(t, err)
	require.Equal(t, breadthFirst, collected)

	traversed := 0
	collected = []string{}
	err = walky.Walk(&root, func(current, parent *yaml.Node, pos int, opts *walky.WalkOptions) (walky.WalkStatus, error) {
		traversed++
		if parent != nil && parent.Kind != yaml.MappingNode {
			return walky.WalkPrune, nil
		}
		if current.Value != "e" {
			return walky.WalkDepthFirst, nil

		}
		err := walky.Walk(parent.Content[pos+1], walky.ScalarValuesWalker(func(n *yaml.Node) error {
			collected = append(collected, n.Value)
			return nil
		}))
		return walky.WalkExit, err
	})
	require.NoError(t, err)
	require.Equal(t, []string{"6", "7"}, collected)
	require.Equal(t, 8, traversed)

	collected = []string{}
	err = walky.Walk(&root, walky.StringWalker("e", func(n *yaml.Node) error {
		return walky.Walk(n, walky.ScalarValuesWalker(func(n *yaml.Node) error {
			collected = append(collected, n.Value)
			return nil
		}))
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"6", "7"}, collected)

	collected = []string{}
	err = walky.Walk(&root, walky.StringWalker("i", func(n *yaml.Node) error {
		return walky.Walk(n, walky.ScalarValuesWalker(func(n *yaml.Node) error {
			collected = append(collected, n.Value)
			return nil
		}))
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"14", "15"}, collected)
	collected = []string{}

	collected = []string{}
	err = walky.Walk(&root, walky.StringWalker("h", func(n *yaml.Node) error {
		return walky.Walk(n, walky.ScalarValuesWalker(func(n *yaml.Node) error {
			collected = append(collected, n.Value)
			return nil
		}))
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"10", "11", "12", "13", "14", "15"}, collected)

	collected = []string{}
	err = walky.Walk(&root, walky.StringWalker("h", func(n *yaml.Node) error {
		return walky.Walk(n, walky.ScalarValuesWalker(func(n *yaml.Node) error {
			collected = append(collected, n.Value)
			return nil
		}))
	}), walky.WithBreadthFirst())
	require.NoError(t, err)
	require.Equal(t, []string{"12", "13", "14", "15", "10", "11"}, collected)

	collected = []string{}
	err = walky.Walk(&root, walky.StringWalker("h", func(n *yaml.Node) error {
		return walky.Walk(n, walky.ScalarValuesWalker(func(n *yaml.Node) error {
			collected = append(collected, n.Value)
			return nil
		}))
	}), walky.WithFirstOnly())
	require.NoError(t, err)
	require.Equal(t, []string{"10", "11"}, collected)

	collected = []string{}
	err = walky.Walk(&root, walky.StringWalker("h", func(n *yaml.Node) error {
		return walky.Walk(n, walky.ScalarValuesWalker(func(n *yaml.Node) error {
			collected = append(collected, n.Value)
			return nil
		}), walky.WithMaxDepth(0))
	}), walky.WithBreadthFirst(), walky.WithMaxDepth(0))
	require.NoError(t, err)
	require.Equal(t, []string{"12", "13"}, collected)

	collected = []string{}
	err = walky.Walk(&root, walky.IndexWalker(1, func(n *yaml.Node) error {
		collected = append(collected, n.Value)
		return nil
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"2", "5", "7", "9", "11", "13"}, collected)
}

func TestWalkPath(t *testing.T) {
	doc := HereBytes(`
	a: [1,2,3]
	b: 
		c: [4,5]
		d: 
			e: [6, 7]
		f: [8,9]
	g:
		h: [10, 11]
		e: [16, 17]
	h: 
	 - 12
	 - 13
	 - i: [14, 15]
	`)
	var root yaml.Node
	err := yaml.Unmarshal(doc, &root)
	require.NoError(t, err)

	matchFound := false
	expectedInt := func(expected int) walky.NodeFunc {
		matchFound = false
		return func(node *yaml.Node) error {
			got, err := strconv.Atoi(node.Value)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			matchFound = true
			return nil
		}
	}
	err = walky.WalkPathMatchers(&root, expectedInt(2),
		walky.StringMatcher("a"),
		walky.IndexMatcher(1),
	)
	require.NoError(t, err)
	require.True(t, matchFound)

	err = walky.WalkPathMatchers(&root, expectedInt(6),
		walky.StringMatcher("b"),
		walky.StringMatcher("d"),
		walky.StringMatcher("e"),
		walky.IndexMatcher(0),
	)
	require.NoError(t, err)
	require.True(t, matchFound)

	err = walky.WalkPathMatchers(&root, expectedInt(15),
		walky.StringMatcher("h"),
		walky.IndexMatcher(2),
		walky.StringMatcher("i"),
		walky.IndexMatcher(1),
	)
	require.NoError(t, err)
	require.True(t, matchFound)

	expectSequence := func(fns ...walky.NodeFunc) walky.NodeFunc {
		return func(n *yaml.Node) error {
			var fn walky.NodeFunc
			fn, fns = fns[0], fns[1:]
			return fn(n)
		}
	}
	err = walky.WalkPathMatchers(&root, expectSequence(expectedInt(7), expectedInt(17)),
		walky.AnyMatcher(),
		walky.StringMatcher("e"),
		walky.IndexMatcher(1),
	)
	require.NoError(t, err)
	require.True(t, matchFound)

	err = walky.WalkPathMatchers(&root, expectSequence(expectedInt(17), expectedInt(7)),
		walky.AnyMatcher(walky.WithBreadthFirst()),
		walky.StringMatcher("e"),
		walky.IndexMatcher(1),
	)
	require.NoError(t, err)
	require.True(t, matchFound)

	err = walky.WalkPath(&root, expectedInt(2), "a", 1)
	require.NoError(t, err)
	require.True(t, matchFound)

	err = walky.WalkPath(&root, expectedInt(6), "b", "d", "e", 0)
	require.NoError(t, err)
	require.True(t, matchFound)

	err = walky.WalkPath(&root, expectedInt(15), "h", 2, "i", 1)
	require.NoError(t, err)
	require.True(t, matchFound)

	err = walky.WalkPath(&root, expectedInt(2), "a", 1.0)
	require.EqualError(t, err, "Unable to make PathMatcher from type float64 (1)")
	require.False(t, matchFound)
}

func src() string {
	_, file, line, _ := runtime.Caller(1)
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

type selectors []interface{}

func TestWalkAssign(t *testing.T) {
	for _, tt := range []struct {
		Name     string
		Input    string
		Select   selectors
		Update   interface{}
		Expected string
	}{{
		Name: src(),
		Input: Here(`
			key: value
		`),
		Select: selectors{"key"},
		Update: "new value",
		Expected: Here(`
			key: new value
		`),
	}, {
		Name: src(),
		Input: Here(`
			key: value # postfix
		`),
		Select: selectors{"key"},
		Update: "new value",
		Expected: Here(`
			key: new value # postfix
		`),
	}, {
		Name: src(),
		Input: Here(`
			# header
			foo: bar
			key: value # line comment
			# footer
			bin: baz
		`),
		Select: selectors{"key"},
		Update: "new\nvalue",
		Expected: Here(`
			# header
			foo: bar
			key: |- # line comment
				new
				value
			# footer
			bin: baz
		`),
	}, {
		Name: src(),
		Input: Here(`
			a:
				b:
					c: [1,2,3]
			d:
				e:
					- 4 # four
					- 5 # five
		`),
		Select: selectors{"a", "b", "c", 1},
		Update: 42,
		Expected: Here(`
			a:
				b:
					c: [1, 42, 3]
			d:
				e:
					- 4 # four
					- 5 # five
		`),
	}, {
		Name: src(),
		Input: Here(`
			a:
				b:
					c: [1,2,3]
			d:
				e:
					- 4 # four
					- 5 # five
		`),
		Select: selectors{"d", "e", 1},
		Update: "five",
		Expected: Here(`
			a:
				b:
					c: [1, 2, 3]
			d:
				e:
					- 4 # four
					- five # five
		`),
	}} {
		t.Run(tt.Name, func(t *testing.T) {
			var root yaml.Node
			err := yaml.Unmarshal([]byte(tt.Input), &root)
			require.NoError(t, err)

			err = walky.WalkPath(&root, func(node *yaml.Node) error {
				update, err := walky.ToNode(tt.Update)
				require.NoError(t, err)
				walky.AssignNode(node, update)
				return nil
			}, tt.Select...)
			require.NoError(t, err)

			got, err := yaml.Marshal(&root)
			require.NoError(t, err)
			require.Equal(t, tt.Expected, string(got))
		})
	}
}

func TestHasKey(t *testing.T) {
	doc := HereBytes(`
		key: val
	`)
	var root yaml.Node
	err := yaml.Unmarshal(doc, &root)
	require.NoError(t, err)

	found := walky.HasKey(&root, "key")
	require.True(t, found)

	keyNode, err := walky.ToNode("key")
	require.NoError(t, err)

	found = walky.HasKey(&root, keyNode)
	require.True(t, found)

	found = walky.HasKey(&root, "nope")
	require.False(t, found)

	keyNode, err = walky.ToNode("nope")
	require.NoError(t, err)

	found = walky.HasKey(&root, keyNode)
	require.False(t, found)
}

func ExampleWalkPath() {
	doc := `# foo is a bar
foo: bar
someMap:
  - someKey: value # line comment
# bin is a baz
bin: baz
`
	var root yaml.Node
	_ = yaml.Unmarshal([]byte(doc), &root)

	// we can choose to sort the map if we want
	sort.Sort(walky.SortableNodeMap(&root))

	// create new *yaml.Node from interface{} value
	newNode, _ := walky.ToNode("new\nvalue")

	// walk the document tree, looking for keys `someMap`, then
	// index 0, then key `someKey`.  That node resulting from
	// the walk is passed to the `func(node *yaml.Node) error`
	// callback, where we reassign the node.
	_ = walky.WalkPath(&root, func(node *yaml.Node) error {
		walky.AssignNode(node, newNode)
		return nil
	}, "someMap", 0, "someKey")

	// we can also insert new keys to maps and slices
	keyNode, _ := walky.ToNode("newkey")
	newNode, _ = walky.ToNode("new value")
	// Add comments via code
	newNode.LineComment = "<-- Look Here"
	_ = walky.AssignMapNode(&root, keyNode, newNode)

	// now we will fetch the sequence under someMap and
	// append a new value to it
	newNode, _ = walky.ToNode(42)
	_ = walky.WalkPath(&root, func(node *yaml.Node) error {
		walky.AppendNode(node, newNode)
		return nil
	}, "someMap")

	newDoc, _ := yaml.Marshal(&root)
	fmt.Println(string(newDoc))
	// Output:
	// # bin is a baz
	// bin: baz
	// # foo is a bar
	// foo: bar
	// newkey: new value # <-- Look Here
	// someMap:
	//     - someKey: |- # line comment
	//         new
	//         value
	//     - 42
}
