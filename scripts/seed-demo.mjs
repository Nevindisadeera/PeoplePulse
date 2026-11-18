// Fills an empty organization with demo data through the REST API, so the
// dashboard has something to show during development.
//
//   node scripts/seed-demo.mjs
//
// Environment: SEED_EMAIL and SEED_PASSWORD (an Organization Admin), and
// optionally API_URL (default http://localhost:8080).
// It refuses to run if the organization already has employees.

const API = (process.env.API_URL ?? "http://localhost:8080") + "/api/v1";
const { SEED_EMAIL, SEED_PASSWORD } = process.env;
if (!SEED_EMAIL || !SEED_PASSWORD) {
  console.error("Set SEED_EMAIL and SEED_PASSWORD to an Organization Admin account.");
  process.exit(1);
}

let token = "";
async function call(method, path, body) {
  const res = await fetch(API + path, {
    method,
    headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = res.status === 204 ? null : await res.json().catch(() => null);
  if (!res.ok) throw new Error(`${method} ${path} -> ${res.status} ${JSON.stringify(data?.error ?? data)}`);
  return data;
}

// Small deterministic generator so every run produces the same organization.
let state = 20261118;
const random = () => ((state = (state * 1664525 + 1013904223) % 4294967296) / 4294967296);
const pick = (list) => list[Math.floor(random() * list.length)];
const iso = (date) => date.toISOString().slice(0, 10);
const today = new Date(Date.UTC(new Date().getFullYear(), new Date().getMonth(), new Date().getDate()));
const daysAgo = (n) => new Date(today.getTime() - n * 86400000);

const DEPARTMENTS = [
  ["Engineering", "ENG", 12],
  ["Sales", "SAL", 9],
  ["Operations", "OPS", 8],
  ["Finance", "FIN", 5],
  ["People", "HR", 4],
];
const FIRST = ["Asha", "Ravi", "Nimal", "Kumari", "Dinesh", "Sanduni", "Tharindu", "Ishara", "Kasun", "Nadeesha", "Chamara", "Dilani", "Lahiru", "Hasini", "Ruwan", "Maleesha", "Sahan", "Piumi", "Yasas", "Thilini"];
const LAST = ["Perera", "Silva", "Fernando", "Jayasinghe", "Bandara", "Wickramasinghe", "Gunawardena", "Rajapaksa", "Herath", "Dissanayake"];
const TITLES = { ENG: ["Software Engineer", "QA Engineer", "Tech Lead"], SAL: ["Account Executive", "Sales Associate"], OPS: ["Operations Analyst", "Coordinator"], FIN: ["Accountant", "Finance Analyst"], HR: ["HR Officer", "Recruiter"] };

// Separations completed during the last twelve months: [days ago, type, reason].
const EXITS = [
  [330, "resignation", "better_opportunity"],
  [290, "resignation", "personal"],
  [245, "termination", "performance"],
  [200, "resignation", "relocation"],
  [195, "resignation", "better_opportunity"],
  [160, "resignation", "better_opportunity"],
  [120, "retirement", "retirement"],
  [88, "resignation", "personal"],
  [85, "resignation", "further_studies"],
  [80, "termination", "misconduct"],
  [50, "resignation", "work_environment"],
  [20, "redundancy", "redundancy"],
];

token = (await call("POST", "/auth/login", { email: SEED_EMAIL, password: SEED_PASSWORD })).accessToken;
if ((await call("GET", "/employees?pageSize=1")).total > 0) {
  console.error("This organization already has employees. The demo seed only runs on an empty organization.");
  process.exit(1);
}

// Backdated separations are recorded without approvals or clearance, then the
// organization's own settings are put back.
const settings = (await call("GET", "/organization")).settings;
await call("PUT", "/settings/approval-chain", { approvalChain: [] });
await call("PUT", "/settings/clearance-template", { clearanceTemplate: [] });

try {
  const employees = [];
  let number = 1;
  for (const [name, code, size] of DEPARTMENTS) {
    const dept = await call("POST", "/departments", { name, code });
    let manager = null;
    for (let i = 0; i < size; i++) {
      const fullName = `${pick(FIRST)} ${pick(LAST)}`;
      const employeeNumber = `E${String(number++).padStart(3, "0")}`;
      // The first person in each department is the long-serving head; the rest joined over the last three years.
      const joined = daysAgo(i === 0 ? 1100 + Math.floor(random() * 300) : 380 + Math.floor(random() * 700));
      const employee = await call("POST", "/employees", {
        employeeNumber,
        fullName,
        workEmail: `${fullName.toLowerCase().replace(/\s+/g, ".")}.${employeeNumber.toLowerCase()}@example.com`,
        departmentId: dept.id,
        jobTitle: i === 0 ? `Head of ${name}` : pick(TITLES[code]),
        employmentType: i === size - 1 && size > 5 ? "intern" : pick(["permanent", "permanent", "permanent", "contract", "probation"]),
        joinDate: iso(joined),
        ...(manager ? { managerId: manager } : {}),
      });
      if (i === 0) {
        manager = employee.id;
        await call("PATCH", `/departments/${dept.id}`, { headEmployeeId: employee.id, version: dept.version });
      } else {
        employees.push({ ...employee, deptCode: code });
      }
    }
  }

  // Recent joiners, so the last few months show hiring.
  const depts = (await call("GET", "/departments")).data;
  for (const ago of [345, 338, 300, 262, 255, 250, 214, 180, 176, 150, 143, 139, 110, 78, 75, 70, 40, 36, 12, 9, 5]) {
    const dept = pick(depts);
    const fullName = `${pick(FIRST)} ${pick(LAST)}`;
    const employeeNumber = `E${String(number++).padStart(3, "0")}`;
    await call("POST", "/employees", { employeeNumber, fullName, departmentId: dept.id, jobTitle: "Associate", employmentType: "probation", joinDate: iso(daysAgo(ago)) });
  }

  // Completed separations, spread across departments.
  const leavers = employees.filter((e) => e.employmentType !== "intern").sort(() => random() - 0.5).slice(0, EXITS.length);
  for (const [i, [ago, type, reasonCode]] of EXITS.entries()) {
    const separation = await call("POST", "/separations", {
      employeeId: leavers[i].id,
      type,
      reasonCode,
      submissionDate: iso(daysAgo(ago + 30)),
      requestedLastWorkingDay: iso(daysAgo(ago)),
    });
    await call("POST", `/separations/${separation.id}/complete`, { version: separation.version });
  }
  console.log(`Created ${number - 1} employees in ${DEPARTMENTS.length} departments and ${EXITS.length} completed separations.`);
} finally {
  await call("PUT", "/settings/approval-chain", { approvalChain: settings.approvalChain });
  await call("PUT", "/settings/clearance-template", { clearanceTemplate: settings.clearanceTemplate });
}

// With the real settings back: one resignation waiting for approval and one serving notice.
const staying = (await call("GET", "/employees?status=active&pageSize=100")).data.filter((e) => e.managerId);
const pending = await call("POST", "/separations", { employeeId: staying[0].id, reasonCode: "personal" });
console.log(`Created a pending resignation for ${pending.employee.fullName}.`);
console.log("Done. Set the analytics start date under organization settings to remove the dashboard notice.");
