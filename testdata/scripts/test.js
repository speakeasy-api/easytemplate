require("scripts/include.js");

_.each(["a", "b", "c"], (v) => {
  console.log(v);
});

console.log("Hello easytemplate!");

let values = [1, 2, 3, 4, 5];

let reduced = _.reduce(values, (sum, value) => add(sum, value), 0);

templateFile("templates/test.stmpl", "test.txt", {
  Test: "from test.js",
  Value: multiply(add(reduced, 2), 2),
});
