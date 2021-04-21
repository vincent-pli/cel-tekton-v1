// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package ext contains CEL extension libraries where each library defines a related set of
// constants, functions, macros, or other configuration settings which may not be covered by
// the core CEL spec.
package ext

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/interpreter/functions"

	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

// Strings returns a cel.EnvOption to configure extended functions for string manipulation.
// As a general note, all indices are zero-based.
//
// CharAt
//
// Returns the character at the given position. If the position is negative, or greater than
// the length of the string, the function will produce an error:
//
//     <string>.charAt(<int>) -> <string>
//
// Examples:
//
//     'hello'.charAt(4)  // return 'o'
//     'hello'.charAt(5)  // return ''
//     'hello'.charAt(-1) // error
//
// IndexOf
//
// Returns the integer index of the first occurrence of the search string. If the search string is
// not found the function returns -1.
//
// The function also accepts an optional position from which to begin the substring search. If the
// substring is the empty string, the index where the search starts is returned (zero or custom).
//
//     <string>.indexOf(<string>) -> <int>
//     <string>.indexOf(<string>, <int>) -> <int>
//
// Examples:
//
//     'hello mellow'.indexOf('')         // returns 0
//     'hello mellow'.indexOf('ello')     // returns 1
//     'hello mellow'.indexOf('jello')    // returns -1
//     'hello mellow'.indexOf('', 2)      // returns 2
//     'hello mellow'.indexOf('ello', 2)  // returns 7
//     'hello mellow'.indexOf('ello', 20) // error
//
// LastIndexOf
//
// Returns the integer index at the start of the last occurrence of the search string. If the
// search string is not found the function returns -1.
//
// The function also accepts an optional position which represents the last index to be
// considered as the beginning of the substring match. If the substring is the empty string,
// the index where the search starts is returned (string length or custom).
//
//     <string>.lastIndexOf(<string>) -> <int>
//     <string>.lastIndexOf(<string>, <int>) -> <int>
//
// Examples:
//
//     'hello mellow'.lastIndexOf('')         // returns 12
//     'hello mellow'.lastIndexOf('ello')     // returns 7
//     'hello mellow'.lastIndexOf('jello')    // returns -1
//     'hello mellow'.lastIndexOf('ello', 6)  // returns 1
//     'hello mellow'.lastIndexOf('ello', -1) // error
//
// LowerAscii
//
// Returns a new string where all ASCII characters are lower-cased.
//
// This function does not perform Unicode case-mapping for characters outside the ASCII range.
//
//     <string>.lowerAscii() -> <string>
//
// Examples:
//
//     'TacoCat'.lowerAscii()      // returns 'tacocat'
//     'TacoCÆt Xii'.lowerAscii()  // returns 'tacocÆt xii'
//
// Replace
//
// Returns a new string based on the target, which replaces the occurrences of a search string
// with a replacement string if present. The function accepts an optional limit on the number of
// substring replacements to be made.
//
// When the replacement limit is 0, the result is the original string. When the limit is a negative
// number, the function behaves the same as replace all.
//
//     <string>.replace(<string>, <string>) -> <string>
//     <string>.replace(<string>, <string>, <int>) -> <string>
//
// Examples:
//
//     'hello hello'.replace('he', 'we')     // returns 'wello wello'
//     'hello hello'.replace('he', 'we', -1) // returns 'wello wello'
//     'hello hello'.replace('he', 'we', 1)  // returns 'wello hello'
//     'hello hello'.replace('he', 'we', 0)  // returns 'hello hello'
//
// Split
//
// Returns a list of strings split from the input by the given separator. The function accepts
// an optional argument specifying a limit on the number of substrings produced by the split.
//
// When the split limit is 0, the result is an empty list. When the limit is 1, the result is the
// target string to split. When the limit is a negative number, the function behaves the same as
// split all.
//
//     <string>.split(<string>) -> <list<string>>
//     <string>.split(<string>, <int>) -> <list<string>>
//
// Examples:
//
//     'hello hello hello'.split(' ')     // returns ['hello', 'hello', 'hello']
//     'hello hello hello'.split(' ', 0)  // returns []
//     'hello hello hello'.split(' ', 1)  // returns ['hello hello hello']
//     'hello hello hello'.split(' ', 2)  // returns ['hello', 'hello hello']
//     'hello hello hello'.split(' ', -1) // returns ['hello', 'hello', 'hello']
//
// Substring
//
// Returns the substring given a numeric range corresponding to character positions. Optionally
// may omit the trailing range for a substring from a given character position until the end of
// a string.
//
// Character offsets are 0-based with an inclusive start range and exclusive end range. It is an
// error to specify an end range that is lower than the start range, or for either the start or end
// index to be negative or exceed the string length.
//
//     <string>.substring(<int>) -> <string>
//     <string>.substring(<int>, <int>) -> <string>
//
// Examples:
//
//     'tacocat'.substring(4)    // returns 'cat'
//     'tacocat'.substring(0, 4) // returns 'taco'
//     'tacocat'.substring(-1)   // error
//     'tacocat'.substring(2, 1) // error
//
// Trim
//
// Returns a new string which removes the leading and trailing whitespace in the target string.
// The trim function uses the Unicode definition of whitespace which does not include the
// zero-width spaces. See: https://en.wikipedia.org/wiki/Whitespace_character#Unicode
//
//      <string>.trim() -> <string>
//
// Examples:
//
//     '  \ttrim\n    '.trim() // returns 'trim'
//
// UpperAscii
//
// Returns a new string where all ASCII characters are upper-cased.
//
// This function does not perform Unicode case-mapping for characters outside the ASCII range.
//
//    <string>.upperAscii() -> <string>
//
// Examples:
//
//     'TacoCat'.upperAscii()      // returns 'TACOCAT'
//     'TacoCÆt Xii'.upperAscii()  // returns 'TACOCÆT XII'
func Example() cel.EnvOption {
	return cel.Lib(exampleLib{})
}

type exampleLib struct{}

func (exampleLib) CompileOptions() []cel.EnvOption {
	listString := decls.NewListType(decls.String)

	return []cel.EnvOption{
		cel.Declarations(
			decls.NewFunction("indexOf",
				decls.NewInstanceOverload("liststring_index_of_int",
					[]*exprpb.Type{listString, decls.String},
					decls.Int)),
		),
	}
}

func (exampleLib) ProgramOptions() []cel.ProgramOption {
	return []cel.ProgramOption{
		cel.Functions(
			&functions.Overload{
				Operator: "indexOf",
				Binary:   callInListStrStrOutInt(indexOf),
			},
		),
	}
}

func indexOf(strList []string, item string) (int, error) {
	for index, value := range strList {
		if value == item {
			return index, nil
		}
	}

	return -1, nil
}
