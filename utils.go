package walky

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

func unwrapDocument(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) < 1 {
			return node // empty document
		}
		return node.Content[0]
	}
	return node
}

func ToNode(val interface{}) (*yaml.Node, error) {
	node := yaml.Node{}
	switch v := val.(type) {
	case yaml.Node:
		return unwrapDocument(&v), nil
	case *yaml.Node:
		return unwrapDocument(v), nil
	case string:
		node.SetString(v)
		return &node, nil
	}
	err := node.Encode(val)
	if err != nil {
		return nil, err
	}
	return &node, nil
}

type sortableNodeMap []*yaml.Node

func SortableNodeMap(mapNode *yaml.Node) sort.Interface {
	mapNode = unwrapDocument(mapNode)
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
	if a.Kind == yaml.AliasNode {
		a = a.Alias
	}
	if b.Kind == yaml.AliasNode {
		b = b.Alias
	}
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
		// FIXME maps should be allowed to be random order
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
	mapNode = unwrapDocument(mapNode)
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
	mapNode = unwrapDocument(mapNode)
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
