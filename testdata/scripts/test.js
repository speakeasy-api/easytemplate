require("scripts/include.js");

let values = [1, 2, 3, 4, 5];

let reduced = _.reduce(values, (sum, value) => add(sum, value), 0);

templateFile("templates/test.stmpl", "test.txt", {
  Test: "from test.js",
  Value: multiply(add(reduced, 2), 2),
});
