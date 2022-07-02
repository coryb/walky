package walky

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// UnwrapDocument removes the root node if and only if the root node has a Kind
// value of `yaml.DocumentNode`.  It returns the first child node of the
// root, which is typically the document data.
func UnwrapDocument(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) < 1 {
			return node // empty document
		}
		return node.Content[0]
	}
	return node
}

func NewDocumentNode() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
	}
}

func NewMappingNode() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}
}

func NewSequenceNode() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
	}
}

// NewStringNode creates a new Node with the value of the provided string.
func NewStringNode(value string) *yaml.Node {
	var node yaml.Node
	node.SetString(value)
	return &node
}

// NewBoolNode creates a new Node with the value of the provided bool.
func NewBoolNode(value bool) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!bool",
		Value: strconv.FormatBool(value),
	}
}

// NewIntNode creates a new Node with the value of the provided int64.
func NewIntNode(value int64) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!int",
		Value: strconv.FormatInt(value, 10),
	}
}

// NewFloatNode creates a new Node with the value of the provided float64.
func NewFloatNode(value float64) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!float",
		Value: strconv.FormatFloat(value, 'f', -1, 64),
	}
}

// KindString will return a human-readable string that represents the
// yaml.Kind arguments.
func KindString(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	}
	return "unknown"
}

func ToNode(val interface{}) (*yaml.Node, error) {
	node := yaml.Node{}
	switch v := val.(type) {
	case yaml.Node:
		return UnwrapDocument(&v), nil
	case *yaml.Node:
		return UnwrapDocument(v), nil
	case string:
		node.SetString(v)
		return &node, nil
	}
	content, err := yaml.Marshal(val)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(content, &node)
	if err != nil {
		return nil, err
	}
	return UnwrapDocument(&node), nil
}

type sortableNodeMap []*yaml.Node

func SortableNodeMap(mapNode *yaml.Node) sort.Interface {
	mapNode = UnwrapDocument(mapNode)
	// if node is not a map this becomes a no-op and we
	// will instead sort and empty slice.
	if mapNode.Kind != yaml.MappingNode {
		return sortableNodeMap([]*yaml.Node{})
	}
	return sortableNodeMap(mapNode.Content)
}

func (sm sortableNodeMap) Len() int {
	return len(sm) / 2
}

func (sm sortableNodeMap) Swap(i, j int) {
	iIndex, jIndex := i*2, j*2
	sm[iIndex], sm[iIndex+1], sm[jIndex], sm[jIndex+1] = sm[jIndex], sm[jIndex+1], sm[iIndex], sm[iIndex+1]
}

// Less compares two keys in a yaml Mapping type.  It will compare the kinds,
// tags and content length before comparing the value.  It will not recurse
// into complex types (other than comparign relative size)
func (sm sortableNodeMap) Less(i, j int) bool {
	iIndex, jIndex := i*2, j*2
	if sm[iIndex].Kind != sm[jIndex].Kind {
		return sm[iIndex].Kind < sm[jIndex].Kind
	}
	if sm[iIndex].Tag != sm[jIndex].Tag {
		return sm[iIndex].Tag < sm[jIndex].Tag
	}
	if len(sm[iIndex].Content) != len(sm[jIndex].Content) {
		return len(sm[iIndex].Content) < len(sm[jIndex].Content)
	}

	// FIXME this comparison needs to parse the numeric values to compare
	// correctly
	return sm[iIndex].Value < sm[jIndex].Value
}

func Equal(a *yaml.Node, b *yaml.Node) bool {
	if a == nil || b == nil {
		return false
	}
	a = Indirect(a)
	b = Indirect(b)
	if a.Kind != b.Kind {
		return false
	}
	if a.Tag != b.Tag {
		return false
	}
	if a.Value != b.Value {
		return false
	}
	if len(a.Content) != len(b.Content) {
		return false
	}
	if a.Kind == yaml.MappingNode {
		aContent := make([]*yaml.Node, len(a.Content))
		bContent := make([]*yaml.Node, len(b.Content))
		copy(aContent, a.Content)
		copy(bContent, b.Content)
		sort.Sort(sortableNodeMap(aContent))
		sort.Sort(sortableNodeMap(bContent))
		for i := 0; i < len(aContent); i++ {
			if !Equal(aContent[i], bContent[i]) {
				return false
			}
		}
	} else {
		for i := 0; i < len(a.Content); i++ {
			if !Equal(a.Content[i], b.Content[i]) {
				return false
			}
		}
	}
	return true
}

// AssignNode copies over the structure data from `srcNode` leaving the document
// data alone (comments, line numbers etc are preserved in `destNode`).
func AssignNode(destNode, srcNode *yaml.Node) {
	destNode.Alias = srcNode.Alias
	destNode.Anchor = srcNode.Anchor
	destNode.Content = srcNode.Content
	destNode.Kind = srcNode.Kind
	destNode.Tag = srcNode.Tag
	destNode.Value = srcNode.Value
}

func AssignMapNode(mapNode, keyNode, valNode *yaml.Node) error {
	mapNode = UnwrapDocument(mapNode)
	if mapNode.Kind != yaml.MappingNode {
		return fmt.Errorf("AssignMapNode called on invalid type: %s", mapNode.Tag)
	}

	found := false
	err := WalkPath(mapNode, func(node *yaml.Node) error {
		found = true
		AssignNode(node, valNode)
		return nil
	}, keyNode)
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	// not an update, so insert key alphabetically into the map. If
	// the key is not a scalar, just insert the element at the end.
	insertAt := len(mapNode.Content)
	if keyNode.Kind == yaml.ScalarNode {
		for i := 0; i < len(mapNode.Content); i += 2 {
			if keyNode.Value < mapNode.Content[i].Value {
				insertAt = i
				break
			}
		}
	}
	mapNode.Content = append(mapNode.Content[:insertAt], append([]*yaml.Node{keyNode, valNode}, mapNode.Content[insertAt:]...)...)
	return nil
}

func AppendNode(listNode, valNode *yaml.Node) error {
	if listNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("AppendNode called on invalid type: %s", listNode.Tag)
	}
	listNode.Content = append(listNode.Content, valNode)
	return nil
}

func HasKey(mapNode *yaml.Node, key interface{}) bool {
	mapNode = UnwrapDocument(mapNode)
	if mapNode.Kind != yaml.MappingNode {
		return false
	}
	found := false
	WalkPath(mapNode, func(n *yaml.Node) error {
		found = true
		return nil
	}, key)
	return found
}

func GetKey(mapNode *yaml.Node, key interface{}) (node *yaml.Node) {
	mapNode = UnwrapDocument(mapNode)
	if mapNode.Kind != yaml.MappingNode {
		return nil
	}
	WalkPath(mapNode, func(n *yaml.Node) error {
		node = n
		return nil
	}, key)
	return node
}

// GetIndex returns the index of the target node found in the parent node.  If
// the parent is a MappingNode the index corresponds to the key node (the value
// will be the key node index + 1).   If the parent node is not a SequenceNode
// or a MappingNode, then -1 will be returned. Also if the target node is not
// found then -1 will be returned.
func GetIndex(parent *yaml.Node, target *yaml.Node) int {
	parent = UnwrapDocument(parent)
	if parent.Kind != yaml.MappingNode && parent.Kind != yaml.SequenceNode {
		return -1
	}
	incrementBy := 1
	if parent.Kind == yaml.MappingNode {
		incrementBy = 2
	}
	for i := 0; i < len(parent.Content); i += incrementBy {
		if Equal(parent.Content[i], target) {
			return i
		}
	}
	return -1
}

// GetKeyValue is used to to simplify getting both the key and value nodes
// from the provided MappingNode.  If the key node is not found then the
// returned nodes will both be `nil`
func GetKeyValue(mapNode *yaml.Node, key *yaml.Node) (keyNode *yaml.Node, valueNode *yaml.Node) {
	mapNode = UnwrapDocument(mapNode)
	if mapNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	ix := GetIndex(mapNode, key)
	if ix < 0 {
		return nil, nil
	}
	return mapNode.Content[ix], mapNode.Content[ix+1]
}

// Remove will delete target node from parent node.  If parent is a MappingNode
// then target should correspond to the mapping Key.  If parent is a
// SequenceNode then the target node will be deleted.  Returns true if and only
// if the target was found in the parent.
func Remove(parent *yaml.Node, target *yaml.Node) bool {
	parent = UnwrapDocument(parent)
	ix := GetIndex(parent, target)
	if ix < 0 {
		return false
	}
	if parent.Kind == yaml.MappingNode {
		// delete key and value nodes
		parent.Content = append(parent.Content[:ix], parent.Content[ix+2:]...)
		return true
	}
	parent.Content = append(parent.Content[:ix], parent.Content[ix+1:]...)
	return true
}

// CopyNode will do a deep copy of the src Node and return a copy
func CopyNode(src *yaml.Node) *yaml.Node {
	copied := map[*yaml.Node]*yaml.Node{}
	return copyNode(src, copied)
}

// copyNode will do a deep copy of src, the copied map is used to prevent
// extra work if we have already seen the src node.  This will allow
// the yaml.Node.Alias to continue to point to the same (copied) node.
func copyNode(src *yaml.Node, copied map[*yaml.Node]*yaml.Node) *yaml.Node {
	if src == nil {
		return nil
	}
	if alias, ok := copied[src]; ok {
		return alias
	}
	cp := *src
	if src.Alias != nil {
		cp.Alias = copyNode(src.Alias, copied)
	}
	cp.Content = make([]*yaml.Node, 0, len(src.Content))
	for _, c := range src.Content {
		cp.Content = append(cp.Content, copyNode(c, copied))
	}
	copied[src] = &cp
	return &cp
}

// ShallowCopyNode will do a shallow copy of the src Node and return a copy.
// Any Contents and Alias will not be copied.
func ShallowCopyNode(src *yaml.Node) *yaml.Node {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

// ReadFile is a helper function to read a file and return a yaml.Node
func ReadFile(filepath string) (*yaml.Node, error) {
	fh, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	dec := yaml.NewDecoder(fh)
	var node yaml.Node
	if err = dec.Decode(&node); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return &node, nil
}

// Indirect will return the aliased node if this node is an alias,
// otherwise it will return the original node.
func Indirect(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

// RangerFunc is the callback used to iterate over a map via RangeMap.  The
// function is called for each key/value pair found in the map.  If an error is
// returned then RangeMap will immediately return the error.  The bool return
// value can be used to stop the iteration (when false) without an error.
type RangerFunc func(key, value *yaml.Node) (bool, error)

// RangeMap will iterate over `node`, calling the range func for each key/value
// pair. An error will be returned if the node is not a mapping node (or an
// alias referencing a mapping node).  An error will be returned unless the
// node.Content is an even length.  If the range func returns an error it
// will be returned immediately.
func RangeMap(node *yaml.Node, f RangerFunc) error {
	node = Indirect(node)
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected node kind %s, got %s", KindString(yaml.MappingNode), KindString(node.Kind))
	}
	// node content is pairs of [key, value], so must be even length
	if len(node.Content)%2 != 0 {
		return fmt.Errorf("unexpected node content length %d, must be even", len(node.Content))
	}
	l := len(node.Content)
	for i := 0; i < l; i += 2 {
		cont, err := f(node.Content[i], node.Content[i+1])
		if err != nil {
			return err
		}
		if !cont {
			return nil
		}
	}
	return nil
}
