package ajson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func ExampleMarshal() {
	data := []byte(`[{"latitude":1,"longitude":2},{"other":"value"},null,{"internal":{"name": "unknown", "longitude":22, "latitude":11}}]`)
	root := Must(Unmarshal(data))
	locations, _ := root.JSONPath("$..[?(@.latitude && @.longitude)]")
	for _, location := range locations {
		name := fmt.Sprintf("At [%v, %v]", location.MustKey("latitude").MustNumeric(), location.MustKey("longitude").MustNumeric())
		_ = location.AppendObject("name", StringNode("", name))
	}
	result, _ := Marshal(root)
	fmt.Printf("%s", result)
	// JSON Output:
	// [
	// 	{
	// 		"latitude":1,
	// 		"longitude":2,
	// 		"name":"At [1, 2]"
	// 	},
	// 	{
	// 		"other":"value"
	// 	},
	// 	null,
	// 	{
	// 		"internal":{
	// 			"name":"At [11, 22]",
	// 			"longitude":22,
	// 			"latitude":11
	// 		}
	// 	}
	// ]
}

func TestMarshal_Primitive(t *testing.T) {
	tests := []struct {
		name string
		node *Node
	}{
		{
			name: "null",
			node: NullNode(""),
		},
		{
			name: "true",
			node: BoolNode("", true),
		},
		{
			name: "false",
			node: BoolNode("", false),
		},
		{
			name: `"string"`,
			node: StringNode("", "string"),
		},
		{
			name: `"one \"encoded\" string"`,
			node: StringNode("", `one "encoded" string`),
		},
		{
			name: `"spec.symbols: \r\n\t; UTF-8: 😹; \u2028 \u0000"`,
			node: StringNode("", "spec.symbols: \r\n\t; UTF-8: 😹; \u2028 \000"),
		},
		{
			name: "100500",
			node: NumericNode("", 100500),
		},
		{
			name: "100.5",
			node: NumericNode("", 100.5),
		},
		{
			name: "[1,2,3]",
			node: ArrayNode("", []*Node{
				NumericNode("0", 1),
				NumericNode("2", 2),
				NumericNode("3", 3),
			}),
		},
		{
			name: `{"foo":"bar"}`,
			node: ObjectNode("", map[string]*Node{
				"foo": StringNode("foo", "bar"),
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := Marshal(test.node)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			} else if string(value) != test.name {
				t.Errorf("wrong result: '%s', expected '%s'", value, test.name)
			}
		})
	}
}

func TestMarshal_Unparsed(t *testing.T) {
	node := Must(Unmarshal([]byte(`{"foo":"bar"}`)))
	node.borders[1] = 0 // broken borders
	_, err := Marshal(node)
	if err == nil {
		t.Errorf("expected error")
	} else if current, ok := err.(Error); !ok {
		t.Errorf("unexpected error type: %T %s", err, err)
	} else if current.Error() != "not parsed yet" {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestMarshal_Encoded(t *testing.T) {
	base := `"one \"encoded\" string"`
	node := Must(Unmarshal([]byte(base)))

	value, err := Marshal(node)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	} else if string(value) != base {
		t.Errorf("wrong result: '%s', expected '%s'", value, base)
	}
}

func TestMarshal_Errors(t *testing.T) {
	tests := []struct {
		name string
		node func() (node *Node)
	}{
		{
			name: "nil",
			node: func() (node *Node) {
				return
			},
		},
		{
			name: "broken",
			node: func() (node *Node) {
				node = Must(Unmarshal([]byte(`{}`)))
				node.borders[1] = 0
				return
			},
		},
		{
			name: "Numeric",
			node: func() (node *Node) {
				return valueNode(nil, "", Numeric, false)
			},
		},
		{
			name: "String",
			node: func() (node *Node) {
				return valueNode(nil, "", String, false)
			},
		},
		{
			name: "Bool",
			node: func() (node *Node) {
				return valueNode(nil, "", Bool, 1)
			},
		},
		{
			name: "Array_1",
			node: func() (node *Node) {
				node = ArrayNode("", nil)
				node.children["1"] = NullNode("1")
				return
			},
		},
		{
			name: "Array_2",
			node: func() (node *Node) {
				return ArrayNode("", []*Node{valueNode(nil, "", Bool, 1)})
			},
		},
		{
			name: "Object",
			node: func() (node *Node) {
				return ObjectNode("", map[string]*Node{"key": valueNode(nil, "key", Bool, 1)})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := Marshal(test.node())
			if err == nil {
				t.Errorf("expected error")
			} else if len(value) != 0 {
				t.Errorf("wrong result")
			}
		})
	}
}

func TestMarshal_String(t *testing.T) {
	formatBody := `{"field1":{"sub_field":"a","sub2":"b"},"field2":[1,2,4]}`
	//	formatBody := `{
	//     "field1": {
	//        "sub_field": "a",
	//        "sub2": "b"
	//    },
	//    "field2": [
	//        1,
	//        2,
	//        4
	//    ]
	//}`
	fieldName := "field1.sub_field"
	jsonPath := fmt.Sprintf("$.%s", fieldName)

	rootNode, err := Unmarshal([]byte(formatBody))

	commands, err := ParseJSONPath(jsonPath)
	if err != nil {
		return
	}

	nodes, err := ApplyJSONPath(rootNode, commands)
	if err != nil {
		return
	}

	fmt.Println(len(nodes))
	node := nodes[0]
	fmt.Println(node.Key())
	fmt.Println(string(node.Source()))
	oldStr := fmt.Sprintf(`"%s":%s`, node.Key(), node.Source())
	newStr := fmt.Sprintf(`"%s":%s`, fieldName, node.Source())

	byteArr, err := Marshal(rootNode)
	fmt.Println(err)
	fmt.Println(string(byteArr))

	body := strings.Replace(formatBody, oldStr, newStr, 1)
	fmt.Println(body)

}

func TestMarshal_Array(t *testing.T) {
	formatBody := `{"field1":{"sub_field":"a","sub2":"b"},"field2":[1,2,4]}`

	fieldName := "field2"
	jsonPath := fmt.Sprintf("$.%s", fieldName)

	rootNode, err := Unmarshal([]byte(formatBody))

	commands, err := ParseJSONPath(jsonPath)
	if err != nil {
		return
	}

	nodes, err := ApplyJSONPath(rootNode, commands)
	if err != nil {
		return
	}

	fmt.Println(len(nodes))
	node := nodes[0]
	fmt.Println(node.Key())
	fmt.Println(string(node.Source()))
	oldStr := fmt.Sprintf(`"%s":%s`, node.Key(), node.Source())
	newStr := fmt.Sprintf(`"%s":%s`, fieldName, node.Source())

	byteArr, err := Marshal(rootNode)
	fmt.Println(err)
	fmt.Println(string(byteArr))

	body := strings.Replace(formatBody, oldStr, newStr, 1)
	fmt.Println(body)

}

func TestMarshal_Array_Ele(t *testing.T) {
	formatBody := `{"field1":{"sub_field":"a","sub2":"b"},"field2":[1,2,4]}`

	fieldName := "field2[0]"
	jsonPath := fmt.Sprintf("$.%s", fieldName)

	rootNode, err := Unmarshal([]byte(formatBody))

	commands, err := ParseJSONPath(jsonPath)
	if err != nil {
		return
	}

	nodes, err := ApplyJSONPath(rootNode, commands)
	if err != nil {
		return
	}

	fmt.Println(len(nodes))
	node := nodes[0]
	fmt.Println(node.Key())
	fmt.Println(string(node.Source()))
	key := node.Key()
	source := node.Source()
	if key == "" {
		key = node.Parent().Key()
		source = node.Parent().Source()
	}
	oldStr := fmt.Sprintf(`"%s":%s`, key, source)
	newStr := fmt.Sprintf(`"%s":%s`, fieldName, source)

	byteArr, err := Marshal(rootNode)
	fmt.Println(err)
	fmt.Println(string(byteArr))

	body := strings.Replace(formatBody, oldStr, newStr, 1)
	fmt.Println(body)

}

func Test_getReqRespBodyField2LineNumMap(t *testing.T) {
	bodyStr := `{"field1":{"sub_field":"a","sub2":"b"},"field2":[1,2,4],"field3":[{"sub_field":"a","sub2":"b"},{"sub_field":"a","sub2":"b"}]}`
	//fieldNames := []string{"field3[1].sub_field"}
	fieldNames := []string{"field3[0].sub_field"}

	for i := 0; i < 100; i++ {
		result, _ := getReqRespBodyField2LineNumMap(bodyStr, fieldNames)
		fmt.Println(result)
	}
}

const (
	NewVarTemplate = "##var%d"
)

var RegNewVar = regexp.MustCompile("##var(\\d+?)")

func getReqRespBodyField2LineNumMap(bodyStr string, fieldNames []string) (map[string]int, error) {
	rootNode, err := Unmarshal([]byte(bodyStr))
	if err != nil {
		fmt.Println("")
		return nil, err
	}

	newVarStr2FieldNameMap := make(map[string]string)
	for i, fieldName := range fieldNames {
		jsonPath := fmt.Sprintf("$.%s", fieldName)

		commands, err := ParseJSONPath(jsonPath)
		if err != nil {
			fmt.Println("ajson.ParseJSONPath exception")
			continue
		}

		nodes, err := ApplyJSONPath(rootNode, commands)
		if err != nil {
			fmt.Println("ajson.ApplyJSONPath exception")
			continue
		}

		if len(nodes) < 1 {
			fmt.Println("json parse empty node",
				bodyStr, jsonPath)
			continue
		}

		firstNode := nodes[0]
		notEmptyNode := getJsonNotEmptyKeyNode(firstNode)
		oldKey := notEmptyNode.Key()

		varStr := fmt.Sprintf(NewVarTemplate, i)
		newKey := oldKey + varStr
		if notEmptyNode != nil {
			notEmptyNode.key = &newKey
		}

		//if notEmptyNode.parent != nil {
		//	pNode := notEmptyNode.parent
		//	delete(pNode.children, oldKey)
		//	pNode.children[newKey] = notEmptyNode
		//}

		newVarStr2FieldNameMap[varStr] = fieldName
	}

	byteArr, _ := MarshalNewKey(rootNode)
	bodyStrBak := string(byteArr)
	//fmt.Println(bodyStrBak)

	//格式化json
	var formatBodyStrBak bytes.Buffer
	_ = json.Indent(&formatBodyStrBak, []byte(bodyStrBak), "", "    ")

	//定位字段所在行
	fieldName2LineNumMap := make(map[string]int)
	bodyLines := strings.Split(formatBodyStrBak.String(), "\n")
	for i, bodyLine := range bodyLines {
		matchArr := RegNewVar.FindStringSubmatch(bodyLine)
		if matchArr == nil || len(matchArr) < 1 {
			continue
		}

		varIdStr := matchArr[1]
		varId, err := strconv.Atoi(varIdStr)
		if err != nil {
			fmt.Println("strconv.Atoi exception", varIdStr, err)
			continue
		}

		varStr := fmt.Sprintf(NewVarTemplate, varId)
		fieldName, ok := newVarStr2FieldNameMap[varStr]
		if ok {
			fieldName2LineNumMap[fieldName] = i + 1
		}
	}

	return fieldName2LineNumMap, nil
}

func getJsonNotEmptyKeyNode(node *Node) *Node {
	if node == nil {
		return nil
	}

	key := node.Key()
	if key == "" {
		return getJsonNotEmptyKeyNode(node.Parent())
	}
	return node
}

func Test_printBorder(t *testing.T) {
	bodyStr := `{"field1":{"sub_field":"a","sub2":"b"},"field2":[1,2,4],"field3":[{"sub_field":"a","sub2":"b"},{"sub_field":"a","sub2":"b"}]}`

	rootNode, _ := Unmarshal([]byte(bodyStr))
	printBorder(rootNode)
}

func printBorder(node *Node) {
	if node != nil {
		fmt.Println(node.Key(), ": ", node.borders)
	}
	children := node.children
	for _, cNode := range children {
		printBorder(cNode)
	}
}

func Test_m(t *testing.T) {
	for i := 0; i < 50; i++ {
		fmt.Println(test())
	}
}

func test() string {
	bodyStr := `{"PayoutRuleID":"7301896625109403654","PayoutRule":{"Tiers":[{"Tier":1,"Amount":"300","RankStart":1,"RankEnd":1,"Label":"","ValueStart":0,"ValueEnd":0,"ImpressionPerClip":"0"},{"Tier":2,"Amount":"100","RankStart":2,"RankEnd":5,"Label":"","ValueStart":0,"ValueEnd":0,"ImpressionPerClip":"0"},{"Tier":3,"Amount":"40","RankStart":6,"RankEnd":10,"Label":"","ValueStart":0,"ValueEnd":0,"ImpressionPerClip":"0"},{"Tier":4,"Amount":"20","RankStart":11,"RankEnd":15,"Label":"","ValueStart":0,"ValueEnd":0,"ImpressionPerClip":"0"}]},"BaseResp":null}`
	respRootNode, _ := Unmarshal([]byte("{}"))

	bodyNode, bodyErr := Unmarshal([]byte(bodyStr))
	if bodyErr == nil {
		_ = respRootNode.AppendObject("body", bodyNode)
	}

	statusCodeNode := NumericNode("status_code", float64(200))
	_ = respRootNode.AppendObject("status_code", statusCodeNode)

	if respRootNode != nil {
		//newRespRootBytes, _ := MarshalNewKey(respRootNode)
		newRespRootBytes, _ := Marshal(respRootNode)
		respStr := string(newRespRootBytes)
		return respStr
	} else {
		return ""
	}
}
