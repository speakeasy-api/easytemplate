function testMethod() {
  return "hello world"
}

registerTemplateFunc("testMethod", testMethod)

templateFile("templates/testMethod.stmpl", "testMethod1.txt", {});

try {
  registerTemplateFunc("testMethod", testMethod)
} catch (e) {
  templateFile("templates/testMethod.stmpl", "testMethod2.txt", {});
}

unregisterTemplateFunc("testMethod")

function newTestMethod() {
  return "overridden test method"
}
registerTemplateFunc("testMethod", newTestMethod)

templateFile("templates/testMethod.stmpl", "testMethod3.txt", {});
