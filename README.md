# walky - Walk YAML
This is a companion to `go-yaml/yaml.v3` to walk and modify abstract `*yaml.Node` trees.

Example:

```go
package main
import (
	"fmt"

	"github.com/coryb/walky"
	"gopkg.in/yaml.v3"
)

func main() {
	doc := `# foo is a bar
foo: bar
someMap:
  - someKey: value # line comment
# bin is a baz
bin: baz
`
	var root yaml.Node
	err := yaml.Unmarshal([]byte(doc), &root)
	if err != nil {
		panic(err)
	}

	// we can choose to sort the map if we want
	sort.Sort(walky.SortableNodeMap(&root))

	// create new *yaml.Node from interface{} value
	newNode, err := walky.ToNode("new\nvalue")
	if err != nil {
		panic(err)
	}

	// walk the document tree, looking for keys `someMap`, then
	// index 0, then key `someKey`.  That node resulting from
	// the walk is passed to the `func(node *yaml.Node) error`
	// callback, where we reassign the node.
	err = walky.WalkPath(&root, func(node *yaml.Node) error {
		walky.AssignNode(node, newNode)
		return nil
	}, "someMap", 0, "someKey")
	if err != nil {
		panic(err)
	}

	// we can also insert new keys to maps and slices
	keyNode, _ := walky.ToNode("newkey")
	newNode, _ = walky.ToNode("new value")
	// Add comments via code
	newNode.LineComment = "<-- Look Here"
	err = walky.AssignMapNode(&root, keyNode, newNode)
	if err != nil {
		panic(err)
	}

	// now we will fetch the sequence under someMap and
	// append a new value to it
	newNode, _ = walky.ToNode(42)
	err = walky.WalkPath(&root, func(node *yaml.Node) error {
		walky.AppendNode(node, newNode)
		return nil
	}, "someMap")
	if err != nil {
		panic(err)
	}

	newDoc, err := yaml.Marshal(&root)
	if err != nil {
		panic(err)
	}
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
```

