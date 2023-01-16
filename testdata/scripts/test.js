require("scripts/include.js");

templateFile("templates/test.stmpl", "test.txt", {
  Test: "from test.js",
  Value: add(1, 2),
});
