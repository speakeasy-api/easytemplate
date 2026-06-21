require("scripts/include.js");

["a", "b", "c"].forEach((v) => {
  console.log(v);
});

console.log("Hello easytemplate!");

let values = [1, 2, 3, 4, 5];

let reduced = values.reduce((sum, value) => add(sum, value), 0);

templateFile("templates/test.stmpl", "test.txt", {
  Test: "from test.js",
  Value: multiply(add(reduced, 2), 2),
});
templateFile("templates/test5.stmpl", "test5.txt", {});

require("./registerTest.js")