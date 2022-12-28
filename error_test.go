package walky

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDecodeError(t *testing.T) {
	var n yaml.Node
	content := `
myint: "abc"
`
	err := yaml.Unmarshal([]byte(content), &n)
	require.NoError(t, err)

	data := struct {
		MyInt int `yaml:"myint"`
	}{}
	err = n.Decode(&data)
	require.Error(t, err)
	de := ErrFilename(err, "test.yml")
	require.Equal(t, "test.yml:2: cannot unmarshal !!str `abc` into int", de.Error())
}

func TestRangeError(t *testing.T) {
	var n yaml.Node
	content := `
myint: "abc"
`
	err := yaml.Unmarshal([]byte(content), &n)
	require.NoError(t, err)

	notmap := GetKey(&n, "myint")
	err = RangeMap(notmap, func(key, value *yaml.Node) error {
		return nil
	})
	require.Error(t, err)
	de := ErrFilename(err, "test.yml")
	require.Equal(t, `test.yml:2:8 at "abc": expected node kind "mapping", got "scalar"`, de.Error())

	data := struct {
		MyInt int `yaml:"myint"`
	}{}
	err = n.Decode(&data)
	require.Error(t, err)
	de = ErrFilename(err, "test.yml")
	require.Equal(t, "test.yml:2: cannot unmarshal !!str `abc` into int", de.Error())
}

func TestErrorUnwrap(t *testing.T) {
	err := YAMLError{
		Err: io.EOF,
	}

	require.True(t, errors.Is(err, io.EOF))

	err = YAMLError{
		Err: &yaml.TypeError{Errors: []string{"hey"}},
	}

	te := &yaml.TypeError{}
	require.True(t, errors.As(err, &te))
}
