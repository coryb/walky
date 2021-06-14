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
```

