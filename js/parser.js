exports.parseWorkflowFile = parseWorkflowFile;

function parseWorkflowFile(tokens, ofs) {
  const errors = [];
  for (let i = 0; i < tokens.length; i++) {
    if (isa(tokens[i], "ERROR")) errors.push(tokens[i]);
  }
  if (errors.length > 0) return errors;

  let node;
  const ret = [];
  if ((node = parseVersion(tokens, ofs))) {
    ret.push(node);
  }
  while ((node = parseBlock(tokens, ofs))) {
    ret.push(node);
  }
  if (ofs[0] !== tokens.length) {
    return [
      "ERROR",
      `expected top-level block, got ${tokens[ofs[0]][0]}`,
      tokens[ofs[0]][2]
    ];
  }
  return ret;
}

function debugParse(_label, _tokens, _ofs) {
  // console.log(
  //   `parse${_label}: ofs=${_ofs[0]} next=${JSON.stringify(
  //     _tokens.slice(_ofs[0], _ofs[0] + 8)
  //   )}`
  // );
}

function parseVersion(tokens, ofs) {
  debugParse("Version", tokens, ofs);
  if (
    !eq(tokens[ofs[0]], ["BAREWORD", "version"]) ||
    !eq(tokens[ofs[0] + 1], ["OPERATOR", "="]) ||
    tokens[ofs[0] + 2][0] !== "INTEGER"
  )
    return null;

  const node = ["version", tokens[ofs[0] + 2][1]];
  ofs[0] += 3;
  return node;
}

function parseBlock(tokens, ofs) {
  debugParse("Block", tokens, ofs);
  let node;
  if ((node = parseWorkflow(tokens, ofs))) return node;
  return parseAction(tokens, ofs);
}

function parseWorkflow(tokens, ofs) {
  debugParse("Workflow", tokens, ofs);
  return parseTopLevelBlock(tokens, ofs, "workflow");
}

function parseAction(tokens, ofs) {
  debugParse("Action", tokens, ofs);
  return parseTopLevelBlock(tokens, ofs, "action");
}

function parseTopLevelBlock(tokens, ofs, keyword) {
  let myofs = ofs[0];
  if (
    !eq(tokens[myofs], ["BAREWORD", keyword]) ||
    tokens[myofs + 1][0] !== "STRING" ||
    !eq(tokens[myofs + 2], ["OPERATOR", "{"])
  )
    return null;

  const node = [keyword, tokens[myofs + 1][1], {}];
  myofs += 3;
  let child;
  const childofs = [myofs];
  while ((child = parseKVP(tokens, childofs))) {
    node[2][child[0]] = child[1];
  }
  myofs = childofs[0];
  if (!eq(tokens[myofs], ["OPERATOR", "}"])) return null;
  ofs[0] = myofs + 1;
  return node;
}

function parseKVP(tokens, ofs) {
  debugParse("KVP", tokens, ofs);
  if (
    !isa(tokens[ofs[0]], "BAREWORD") ||
    !eq(tokens[ofs[0] + 1], ["OPERATOR", "="])
  )
    return null;
  const key = tokens[ofs[0]][1];
  const childofs = [ofs[0] + 2];
  const val = parseValue(tokens, childofs);
  if (val) {
    ofs[0] = childofs[0];
    return [key, val];
  }
  return null;
}

function parseValue(tokens, ofs) {
  return parseAny(tokens, ofs, [parseString, parseArray, parseObject]);
}

function parseAny(tokens, ofs, func) {
  let node;
  for (let i = 0; i < func.length; i++) {
    if ((node = func[i](tokens, ofs))) return node;
  }
  return null;
}

function parseString(tokens, ofs) {
  if (isa(tokens[ofs[0]], "STRING")) {
    ofs[0]++;
    return tokens[ofs[0] - 1][1];
  }
  return null;
}

function parseArray(tokens, ofs) {
  debugParse("Array", tokens, ofs);
  let myofs = ofs[0];
  if (!eq(tokens[myofs], ["OPERATOR", "["])) return null;
  const ret = [];
  myofs++;
  while (isa(tokens[myofs], "STRING")) {
    ret.push(tokens[myofs][1]);
    myofs++;
    if (!eq(tokens[myofs], ["OPERATOR", ","])) break;
    myofs++;
  }
  if (!eq(tokens[myofs], ["OPERATOR", "]"])) return null;
  ofs[0] = myofs + 1;
  return ret;
}

function parseObject(tokens, ofs) {
  debugParse("Object", tokens, ofs);
  let myofs = ofs[0];
  if (!eq(tokens[myofs], ["OPERATOR", "{"])) return null;
  const ret = {};
  myofs++;
  debugParse("", tokens, [myofs]);
  while (
    isa(tokens[myofs], "BAREWORD") &&
    eq(tokens[myofs + 1], ["OPERATOR", "="]) &&
    isa(tokens[myofs + 2], "STRING")
  ) {
    ret[tokens[myofs][1]] = tokens[myofs + 2][1];
    myofs += 3;
    debugParse("", tokens, [myofs]);
  }
  if (!eq(tokens[myofs], ["OPERATOR", "}"])) return null;
  ofs[0] = myofs + 1;

  return ret;
}

// function parseStringOrArray(tokens, ofs) {
//   debugParse("StringOrArray", tokens, ofs);
//   if (isa(tokens[ofs[0]], "STRING")) {
//     ofs[0]++;
//     return [tokens[ofs[0] - 1][1]];
//   }
//   return parseArray(tokens, ofs);
// }

function eq(a1, a2) {
  if (!a1 || !a2) return false;
  if (a1.length !== 3 || a2.length !== 2) return false;
  for (let i = 0; i < 2; i++) {
    if (a1[i] !== a2[i]) return false;
  }
  return true;
}

function isa(token, type) {
  return token && token[0] === type;
}
