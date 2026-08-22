//! zoro.ai - single-file Rust CLI agentic development orchestrator.
//!
//! Build:
//!   rustc -O zoro.rs -o zoro
//!   # Windows: rustc -O zoro.rs -o zoro.exe
//!
//! Runtime tools:
//!   git, gh, curl, codex
//!
//! Secrets:
//!   GitHub: ZORO_GITHUB_TOKEN, GH_TOKEN, or `gh auth token`
//!   OpenAI: OPENAI_API_KEY

use std::collections::{BTreeMap, HashMap};
use std::env;
use std::fs::{self, File, OpenOptions};
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};
use std::process::{Command, Output, Stdio};
use std::thread;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

const VERSION: &str = "0.1.0";
const CONFIG_PATH: &str = ".zoro/config.yaml";
const LOCK_PATH: &str = ".zoro/runtime/zoro.lock";

#[derive(Debug, Clone)]
struct Config {
    version: u32,
    github: GithubConfig,
    scheduler: SchedulerConfig,
    planning: PlanningConfig,
    automation: AutomationConfig,
    implementation: ImplementationConfig,
    handoff: HandoffConfig,
    behavior: BehaviorConfig,
}

#[derive(Debug, Clone)]
struct GithubConfig {
    owner: String,
    repo: String,
    project_number: u32,
    status_field: String,
    backlog: String,
    ready: String,
    implementing: String,
    review: String,
    done: String,
}

#[derive(Debug, Clone)]
struct SchedulerConfig { enabled: bool, interval: String }
#[derive(Debug, Clone)]
struct PlanningConfig { provider: String, model: String, max_files: usize, max_context_bytes: usize }
#[derive(Debug, Clone)]
struct AutomationConfig { auto_plan: bool, auto_implement: bool }
#[derive(Debug, Clone)]
struct ImplementationConfig {
    provider: String,
    branch_enabled: bool,
    branch_prefix: String,
    validation_enabled: bool,
    validation_commands: Vec<String>,
}
#[derive(Debug, Clone)]
struct HandoffConfig { directory: String }
#[derive(Debug, Clone)]
struct BehaviorConfig {
    max_concurrent_tasks: u32,
    move_to_in_progress_on_implement: bool,
    move_to_review_on_success: bool,
}

#[derive(Debug, Clone)]
struct ProjectItem {
    item_id: String,
    issue_id: Option<String>,
    issue_number: Option<u64>,
    title: String,
    body: String,
    status: String,
    position: usize,
}

#[derive(Debug, Clone)]
struct ProjectMeta {
    project_id: String,
    status_field_id: String,
    options: HashMap<String, String>,
}

#[derive(Debug, Clone)]
struct RelevantFile { path: String, reason: String, expected_change: Option<String> }
#[derive(Debug, Clone)]
struct ProposedChange { file: Option<String>, description: String, risk: Option<String> }
#[derive(Debug, Clone)]
struct AcceptanceCriterion { criterion: String, validation: Option<String> }
#[derive(Debug, Clone)]
struct HandoffPlan {
    summary: String,
    objective: String,
    assumptions: Vec<String>,
    relevant_files: Vec<RelevantFile>,
    proposed_changes: Vec<ProposedChange>,
    preparation: Vec<String>,
    implementation_steps: Vec<String>,
    validation_steps: Vec<String>,
    risks: Vec<String>,
    acceptance_criteria: Vec<AcceptanceCriterion>,
}

#[derive(Debug)]
struct AppError { kind: &'static str, message: String }
type AppResult<T> = Result<T, AppError>;

impl AppError {
    fn new(kind: &'static str, message: impl Into<String>) -> Self { Self { kind, message: message.into() } }
}

impl std::fmt::Display for AppError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result { write!(f, "{}: {}", self.kind, self.message) }
}

impl From<io::Error> for AppError {
    fn from(e: io::Error) -> Self { AppError::new("IOError", e.to_string()) }
}

// Minimal JSON value/parser. Keeps this program dependency-free.
#[derive(Debug, Clone)]
enum Json { Null, Bool(bool), Num(f64), Str(String), Arr(Vec<Json>), Obj(BTreeMap<String, Json>) }

impl Json {
    fn get(&self, key: &str) -> Option<&Json> { match self { Json::Obj(m) => m.get(key), _ => None } }
    fn as_str(&self) -> Option<&str> { match self { Json::Str(s) => Some(s), _ => None } }
    fn as_arr(&self) -> Option<&[Json]> { match self { Json::Arr(v) => Some(v), _ => None } }
    fn as_u64(&self) -> Option<u64> { match self { Json::Num(n) if *n >= 0.0 => Some(*n as u64), _ => None } }
}

struct JsonParser<'a> { b: &'a [u8], i: usize }
impl<'a> JsonParser<'a> {
    fn new(s: &'a str) -> Self { Self { b: s.as_bytes(), i: 0 } }
    fn parse(mut self) -> AppResult<Json> { self.ws(); let v = self.value()?; self.ws(); if self.i != self.b.len() { return Err(AppError::new("JsonError", "trailing JSON data")); } Ok(v) }
    fn ws(&mut self) { while self.i < self.b.len() && self.b[self.i].is_ascii_whitespace() { self.i += 1; } }
    fn value(&mut self) -> AppResult<Json> {
        self.ws();
        if self.i >= self.b.len() { return Err(AppError::new("JsonError", "unexpected end")); }
        match self.b[self.i] {
            b'{' => self.object(), b'[' => self.array(), b'"' => self.string().map(Json::Str),
            b't' => { self.literal(b"true")?; Ok(Json::Bool(true)) },
            b'f' => { self.literal(b"false")?; Ok(Json::Bool(false)) },
            b'n' => { self.literal(b"null")?; Ok(Json::Null) },
            b'-' | b'0'..=b'9' => self.number(),
            _ => Err(AppError::new("JsonError", format!("unexpected byte at {}", self.i))),
        }
    }
    fn literal(&mut self, lit: &[u8]) -> AppResult<()> { if self.b.get(self.i..self.i+lit.len()) == Some(lit) { self.i += lit.len(); Ok(()) } else { Err(AppError::new("JsonError", "invalid literal")) } }
    fn object(&mut self) -> AppResult<Json> {
        self.i += 1; self.ws(); let mut m = BTreeMap::new();
        if self.peek(b'}') { self.i += 1; return Ok(Json::Obj(m)); }
        loop {
            self.ws(); let k = self.string()?; self.ws(); self.expect(b':')?; let v = self.value()?; m.insert(k, v); self.ws();
            if self.peek(b'}') { self.i += 1; break; } self.expect(b',')?;
        }
        Ok(Json::Obj(m))
    }
    fn array(&mut self) -> AppResult<Json> {
        self.i += 1; self.ws(); let mut v = Vec::new();
        if self.peek(b']') { self.i += 1; return Ok(Json::Arr(v)); }
        loop { v.push(self.value()?); self.ws(); if self.peek(b']') { self.i += 1; break; } self.expect(b',')?; }
        Ok(Json::Arr(v))
    }
    fn string(&mut self) -> AppResult<String> {
        self.expect(b'"')?; let mut out = String::new();
        while self.i < self.b.len() {
            let c = self.b[self.i]; self.i += 1;
            match c {
                b'"' => return Ok(out),
                b'\\' => {
                    if self.i >= self.b.len() { return Err(AppError::new("JsonError", "bad escape")); }
                    let e = self.b[self.i]; self.i += 1;
                    match e {
                        b'"' => out.push('"'), b'\\' => out.push('\\'), b'/' => out.push('/'), b'b' => out.push('\x08'), b'f' => out.push('\x0c'), b'n' => out.push('\n'), b'r' => out.push('\r'), b't' => out.push('\t'),
                        b'u' => {
                            if self.i + 4 > self.b.len() { return Err(AppError::new("JsonError", "short unicode escape")); }
                            let h = std::str::from_utf8(&self.b[self.i..self.i+4]).map_err(|_| AppError::new("JsonError", "unicode escape"))?;
                            let cp = u16::from_str_radix(h,16).map_err(|_| AppError::new("JsonError", "unicode escape"))? as u32; self.i += 4;
                            if let Some(ch) = char::from_u32(cp) { out.push(ch); }
                        }
                        _ => return Err(AppError::new("JsonError", "unknown escape")),
                    }
                }
                _ if c < 0x20 => return Err(AppError::new("JsonError", "control character in string")),
                _ if c < 0x80 => out.push(c as char),
                _ => {
                    self.i -= 1;
                    let s = std::str::from_utf8(&self.b[self.i..]).map_err(|_| AppError::new("JsonError", "utf8"))?;
                    let ch = s.chars().next().ok_or_else(|| AppError::new("JsonError", "utf8"))?; out.push(ch); self.i += ch.len_utf8();
                }
            }
        }
        Err(AppError::new("JsonError", "unterminated string"))
    }
    fn number(&mut self) -> AppResult<Json> {
        let start = self.i;
        while self.i < self.b.len() && matches!(self.b[self.i], b'-'|b'+'|b'.'|b'e'|b'E'|b'0'..=b'9') { self.i += 1; }
        let s = std::str::from_utf8(&self.b[start..self.i]).unwrap_or("");
        let n = s.parse::<f64>().map_err(|_| AppError::new("JsonError", "invalid number"))?; Ok(Json::Num(n))
    }
    fn expect(&mut self, c: u8) -> AppResult<()> { self.ws(); if self.peek(c) { self.i += 1; Ok(()) } else { Err(AppError::new("JsonError", format!("expected '{}'", c as char))) } }
    fn peek(&self, c: u8) -> bool { self.i < self.b.len() && self.b[self.i] == c }
}

fn json_escape(s: &str) -> String {
    let mut out = String::with_capacity(s.len()+16);
    for c in s.chars() {
        match c { '"' => out.push_str("\\\""), '\\' => out.push_str("\\\\"), '\n' => out.push_str("\\n"), '\r' => out.push_str("\\r"), '\t' => out.push_str("\\t"), c if c < ' ' => out.push_str(&format!("\\u{:04x}", c as u32)), _ => out.push(c) }
    }
    out
}

fn parse_json(s: &str) -> AppResult<Json> { JsonParser::new(s).parse() }

fn main() {
    if let Err(e) = run_cli() { eprintln!("\n{}\n", e); std::process::exit(1); }
}

fn run_cli() -> AppResult<()> {
    let mut args: Vec<String> = env::args().skip(1).collect();
    if args.is_empty() { print_help(); return Ok(()); }
    if args[0] == "--version" || args[0] == "-V" { println!("zoro.ai {}", VERSION); return Ok(()); }
    let cmd = args.remove(0);
    match cmd.as_str() {
        "init" => cmd_init(&args),
        "auth" => cmd_auth(),
        "doctor" => cmd_doctor(),
        "board" => cmd_board(),
        "ready" => cmd_ready(),
        "plan" => cmd_plan(args.first().and_then(|x| x.parse().ok())),
        "implement" => cmd_implement(args.first().and_then(|x| x.parse().ok())),
        "run" => cmd_run(args.iter().any(|x| x == "--once")),
        "status" => cmd_status(),
        "config" => cmd_config(),
        "help" | "--help" | "-h" => { print_help(); Ok(()) },
        _ => Err(AppError::new("CliError", format!("unknown command: {cmd}"))),
    }
}

fn print_help() {
    println!(r#"zoro.ai {VERSION}

Single-file Rust agentic development orchestrator.

USAGE:
  zoro <command>

COMMANDS:
  init                 Initialize .zoro/config.yaml and handoff directories
  auth                 Verify GitHub repository and Project access
  doctor               Diagnose local dependencies and configuration
  board                Show project status counts
  ready                Show ordered Ready items
  plan [ISSUE]         Plan top Ready item or a specific issue
  implement [ISSUE]    Implement a ready handoff using Codex
  run [--once]         Poll continuously or run exactly one planning cycle
  status               Show local handoff state
  config               Print effective config with no secrets

ENVIRONMENT:
  ZORO_GITHUB_TOKEN or GH_TOKEN or `gh auth login`
  OPENAI_API_KEY
"#);
}

fn cmd_init(args: &[String]) -> AppResult<()> {
    ensure_git_repo()?;
    fs::create_dir_all(".zoro/runtime")?;
    for s in ["ready","implementing","review","done","failed"] { fs::create_dir_all(format!("handoff/{s}"))?; }
    let (owner, repo) = infer_github_repo().unwrap_or_else(|_| ("OWNER".into(), "REPOSITORY".into()));
    let project_number = args.iter().position(|x| x == "--project").and_then(|i| args.get(i+1)).and_then(|x| x.parse::<u32>().ok()).unwrap_or(1);
    if !Path::new(CONFIG_PATH).exists() {
        let yaml = default_config_yaml(&owner, &repo, project_number);
        fs::write(CONFIG_PATH, yaml)?;
        println!("✓ created {CONFIG_PATH}");
    } else { println!("• {CONFIG_PATH} already exists"); }
    add_gitignore_line(".zoro/runtime/")?;
    println!("✓ handoff directories ready");
    println!("Repository: {owner}/{repo}");
    println!("Project: {project_number}");
    println!("Next: review {CONFIG_PATH}, then run `zoro auth` and `zoro doctor`.");
    Ok(())
}

fn default_config_yaml(owner: &str, repo: &str, project: u32) -> String { format!(r#"version: 1

github:
  owner: {owner}
  repo: {repo}
  project_number: {project}
  status_field: Status
  statuses:
    backlog: Backlog
    ready: Ready
    implementing: In progress
    review: In review
    done: Done

scheduler:
  enabled: true
  interval: 1m

planning:
  provider: openai
  model: gpt-5.6
  max_files: 30
  max_context_bytes: 300000

automation:
  auto_plan: true
  auto_implement: false

implementation:
  provider: codex
  branch:
    enabled: true
    prefix: zoro
  validation:
    enabled: true
    commands:
      - cargo test
      - cargo clippy -- -D warnings

handoff:
  directory: handoff

behavior:
  max_concurrent_tasks: 1
  move_to_in_progress_on_implement: true
  move_to_review_on_success: true
"#) }

fn load_config() -> AppResult<Config> {
    let text = fs::read_to_string(CONFIG_PATH).map_err(|e| AppError::new("ConfigError", format!("cannot read {CONFIG_PATH}: {e}. Run `zoro init`.")))?;
    parse_config(&text)
}

// Purpose-built YAML reader for Zoro's fixed config schema. It intentionally rejects missing essentials.
fn parse_config(s: &str) -> AppResult<Config> {
    let mut scalars: HashMap<String,String> = HashMap::new();
    let mut lists: HashMap<String,Vec<String>> = HashMap::new();
    let mut stack: Vec<(usize,String)> = Vec::new();
    let mut current_list: Option<(usize,String)> = None;
    for raw in s.lines() {
        if raw.trim().is_empty() || raw.trim_start().starts_with('#') { continue; }
        let indent = raw.chars().take_while(|c| *c == ' ').count();
        let line = raw.trim();
        if line.starts_with("- ") {
            if let Some((list_indent, key)) = &current_list {
                if indent > *list_indent { lists.entry(key.clone()).or_default().push(unquote(line[2..].trim())); continue; }
            }
            return Err(AppError::new("ConfigError", format!("unexpected list entry: {line}")));
        }
        current_list = None;
        while stack.last().map(|(i,_)| *i >= indent).unwrap_or(false) { stack.pop(); }
        let (key,val) = line.split_once(':').ok_or_else(|| AppError::new("ConfigError", format!("invalid YAML line: {line}")))?;
        let key = key.trim().to_string(); let val = val.trim();
        let mut path: Vec<String> = stack.iter().map(|(_,k)| k.clone()).collect(); path.push(key.clone()); let full = path.join(".");
        if val.is_empty() { stack.push((indent,key)); current_list = Some((indent,full)); }
        else { scalars.insert(full, unquote(val)); }
    }
    let req = |k:&str| -> AppResult<String> { scalars.get(k).cloned().ok_or_else(|| AppError::new("ConfigError", format!("missing {k}"))) };
    let boolv = |k:&str,d:bool| -> AppResult<bool> { match scalars.get(k).map(|s|s.as_str()) { Some("true")=>Ok(true),Some("false")=>Ok(false),Some(x)=>Err(AppError::new("ConfigError",format!("{k} must be true/false, got {x}"))),None=>Ok(d) } };
    let usizev = |k:&str,d:usize| -> AppResult<usize> { scalars.get(k).map(|x| x.parse().map_err(|_|AppError::new("ConfigError",format!("{k} must be numeric")))).unwrap_or(Ok(d)) };
    let u32v = |k:&str,d:u32| -> AppResult<u32> { scalars.get(k).map(|x| x.parse().map_err(|_|AppError::new("ConfigError",format!("{k} must be numeric")))).unwrap_or(Ok(d)) };
    let cfg = Config {
        version: u32v("version",1)?,
        github: GithubConfig {
            owner:req("github.owner")?, repo:req("github.repo")?, project_number:u32v("github.project_number",0)?, status_field:req("github.status_field")?,
            backlog:req("github.statuses.backlog")?, ready:req("github.statuses.ready")?, implementing:req("github.statuses.implementing")?, review:req("github.statuses.review")?, done:req("github.statuses.done")?,
        },
        scheduler: SchedulerConfig { enabled: boolv("scheduler.enabled",true)?, interval:req("scheduler.interval")? },
        planning: PlanningConfig { provider: scalars.get("planning.provider").cloned().unwrap_or("openai".into()), model:req("planning.model")?, max_files:usizev("planning.max_files",30)?, max_context_bytes:usizev("planning.max_context_bytes",300000)? },
        automation: AutomationConfig { auto_plan:boolv("automation.auto_plan",true)?, auto_implement:boolv("automation.auto_implement",false)? },
        implementation: ImplementationConfig { provider:scalars.get("implementation.provider").cloned().unwrap_or("codex".into()), branch_enabled:boolv("implementation.branch.enabled",true)?, branch_prefix:scalars.get("implementation.branch.prefix").cloned().unwrap_or("zoro".into()), validation_enabled:boolv("implementation.validation.enabled",true)?, validation_commands:lists.get("implementation.validation.commands").cloned().unwrap_or_default() },
        handoff:HandoffConfig { directory:scalars.get("handoff.directory").cloned().unwrap_or("handoff".into()) },
        behavior:BehaviorConfig { max_concurrent_tasks:u32v("behavior.max_concurrent_tasks",1)?, move_to_in_progress_on_implement:boolv("behavior.move_to_in_progress_on_implement",true)?, move_to_review_on_success:boolv("behavior.move_to_review_on_success",true)? },
    };
    validate_config(&cfg)?; Ok(cfg)
}

fn unquote(s:&str)->String { let s=s.trim(); if s.len()>=2 && ((s.starts_with('"')&&s.ends_with('"'))||(s.starts_with('\'')&&s.ends_with('\''))) { s[1..s.len()-1].to_string() } else { s.to_string() } }
fn validate_config(c:&Config)->AppResult<()> {
    if c.version != 1 { return Err(AppError::new("ConfigError", "only config version 1 is supported")); }
    if c.github.project_number == 0 { return Err(AppError::new("ConfigError", "github.project_number must be > 0")); }
    if c.github.owner.trim().is_empty() || c.github.repo.trim().is_empty() { return Err(AppError::new("ConfigError", "github owner/repo must not be empty")); }
    if c.planning.max_files == 0 || c.planning.max_files > 200 { return Err(AppError::new("ConfigError", "planning.max_files must be 1..200")); }
    if c.planning.max_context_bytes < 1024 || c.planning.max_context_bytes > 2_000_000 { return Err(AppError::new("ConfigError", "planning.max_context_bytes must be 1024..2000000")); }
    parse_duration(&c.scheduler.interval)?;
    Ok(())
}

fn parse_duration(s:&str)->AppResult<Duration> {
    if s.len()<2 { return Err(AppError::new("ConfigError", format!("invalid duration: {s}"))); }
    let (n,u)=s.split_at(s.len()-1); let n:u64=n.parse().map_err(|_|AppError::new("ConfigError",format!("invalid duration: {s}")))?;
    if n==0 { return Err(AppError::new("ConfigError", "duration must be > 0")); }
    match u { "s"=>Ok(Duration::from_secs(n)),"m"=>Ok(Duration::from_secs(n*60)),"h"=>Ok(Duration::from_secs(n*3600)),_=>Err(AppError::new("ConfigError",format!("unsupported duration unit: {s}. Use s, m, or h."))) }
}

fn cmd_auth() -> AppResult<()> {
    let c=load_config()?; require_cmd("gh")?; let _=resolve_github_token();
    gh_output(&["api","user","--jq",".login"])?;
    gh_output(&["api",&format!("repos/{}/{}",c.github.owner,c.github.repo),"--jq",".full_name"])?;
    let (_,meta)=load_project(&c)?;
    println!("✓ GitHub authentication"); println!("✓ Repository {}/{}",c.github.owner,c.github.repo); println!("✓ Project #{} ({})",c.github.project_number,meta.project_id); println!("✓ Status field {}",c.github.status_field); Ok(())
}

fn cmd_doctor() -> AppResult<()> {
    let cfg=load_config();
    check("Config valid", cfg.is_ok(), cfg.as_ref().err().map(|e|e.message.as_str()));
    let git=cmd_exists("git"); check("Git executable",git,None); let repo=git && ensure_git_repo().is_ok(); check("Git repository",repo,None);
    if repo { let dirty=!git_clean().unwrap_or(false); if dirty { println!("⚠ Repository clean     dirty working tree"); } else { println!("✓ Repository clean"); } }
    let gh=cmd_exists("gh"); check("GitHub CLI",gh,None);
    if let Ok(c)=&cfg {
        let auth=gh && gh_output(&["api","user","--jq",".login"]).is_ok(); check("GitHub auth",auth,None);
        let project=auth && load_project(c).is_ok(); check("GitHub project",project,None);
    }
    check("OpenAI API key",env::var("OPENAI_API_KEY").map(|x|!x.trim().is_empty()).unwrap_or(false),None);
    check("Codex CLI",cmd_exists("codex"),None); check("curl",cmd_exists("curl"),None);
    let dirs=["handoff/ready","handoff/implementing","handoff/review","handoff/done","handoff/failed",".zoro/runtime"];
    check("Handoff directories",dirs.iter().all(|p|Path::new(p).is_dir()),None);
    Ok(())
}
fn check(name:&str,ok:bool,detail:Option<&str>){ if ok { println!("✓ {name}"); } else if let Some(d)=detail { println!("✗ {name:<20} {d}"); } else { println!("✗ {name}"); } }

fn cmd_board()->AppResult<()> { let c=load_config()?; let (items,_)=load_project(&c)?; for s in [&c.github.backlog,&c.github.ready,&c.github.implementing,&c.github.review,&c.github.done] { println!("{:<14} {}",s,items.iter().filter(|x|x.status==*s).count()); } Ok(()) }
fn cmd_ready()->AppResult<()> { let c=load_config()?; let (items,_)=load_project(&c)?; let ready:Vec<_>=items.iter().filter(|x|x.status==c.github.ready).collect(); println!("Ready\n"); for (i,x) in ready.iter().enumerate(){ match x.issue_number {Some(n)=>println!("{}. #{} {}",i+1,n,x.title),None=>println!("{}. {}",i+1,x.title)} } Ok(()) }

fn cmd_plan(issue:Option<u64>)->AppResult<()> { let c=load_config()?; let path=plan_item(&c,issue)?; println!("✓ Handoff created\n  {}",path.display()); Ok(()) }

fn plan_item(c:&Config,issue:Option<u64>)->AppResult<PathBuf> {
    let (items,_)=load_project(c)?;
    let item=match issue { Some(n)=>items.into_iter().find(|x|x.issue_number==Some(n)).ok_or_else(||AppError::new("ProjectError",format!("issue #{n} not found in project")))?, None=>items.into_iter().find(|x|x.status==c.github.ready).ok_or_else(||AppError::new("ProjectError","no Ready items"))? };
    if let Some(existing)=find_handoff(c,item.issue_number,&item.item_id,false)? { return Err(AppError::new("HandoffError",format!("item already has a handoff: {}",existing.display()))); }
    println!("• Selected {}",display_item(&item)); println!("• Inspecting repository");
    let ctx=collect_context(c,&item)?; println!("• Found {} relevant files",ctx.files.len()); println!("• Creating implementation plan with {}",c.planning.model);
    let plan=openai_plan(c,&item,&ctx)?; let path=render_handoff(c,&item,&plan,&ctx)?; Ok(path)
}

struct RepoContext { files:Vec<(String,String)>, git_status:String, instructions:Vec<(String,String)> }
fn collect_context(c:&Config,item:&ProjectItem)->AppResult<RepoContext>{
    ensure_git_repo()?; let git_status=run_text("git", &["status","--porcelain"])?;
    let mut instructions=Vec::new();
    for p in ["AGENTS.md","CLAUDE.md","README.md","pyproject.toml","package.json","go.mod","Cargo.toml","Dockerfile","docker-compose.yml"] { if Path::new(p).is_file(){ if let Ok(t)=read_bounded(Path::new(p),64_000){instructions.push((p.into(),t));} } }
    let mut candidates=Vec::<String>::new();
    let tracked=run_text("git", &["ls-files"])?;
    let keywords=keywords(&format!("{} {}",item.title,item.body));
    for p in tracked.lines() {
        if excluded_path(p) {continue;} if is_probably_binary_path(p){continue;}
        let lower=p.to_lowercase(); let mut score=0usize; for k in &keywords { if lower.contains(k){score+=3;} }
        if lower.contains("test") {score+=1;} if score>0 {candidates.push(p.to_string());}
    }
    // Content search with rg if available, then deterministic path fallback.
    if cmd_exists("rg") {
        for k in keywords.iter().take(8) {
            if let Ok(o)=Command::new("rg").args(["-l","--hidden","--glob","!.git/**","--glob","!node_modules/**","--glob","!vendor/**","--glob","!.venv/**",k,"."]).output(){
                if o.status.success(){ for p in String::from_utf8_lossy(&o.stdout).lines(){ let p=p.trim_start_matches("./"); if !excluded_path(p)&&!candidates.contains(&p.to_string()){candidates.push(p.to_string());} } }
            }
        }
    }
    candidates.sort(); candidates.dedup();
    let mut files=Vec::new(); let mut bytes=0usize;
    for p in candidates { if files.len()>=c.planning.max_files {break;} let path=Path::new(&p); if !path.is_file(){continue;} if let Ok(t)=read_bounded(path,80_000){ if bytes+t.len()>c.planning.max_context_bytes {break;} bytes+=t.len(); files.push((p,t)); } }
    Ok(RepoContext{files,git_status,instructions})
}
fn keywords(s:&str)->Vec<String>{ let stop=["the","and","for","with","from","this","that","into","add","fix","update","create","implement","should","when","then","user","issue","using"]; let mut v=Vec::new(); for w in s.split(|c:char|!c.is_alphanumeric()&&c!='_') { let w=w.to_lowercase(); if w.len()>=4&&!stop.contains(&w.as_str())&&!v.contains(&w){v.push(w);} } v.truncate(20); v }
fn excluded_path(p:&str)->bool { let p=p.replace('\\',"/"); [".git/","node_modules/","vendor/",".venv/","dist/","build/","coverage/",".env","id_rsa","id_ed25519"].iter().any(|x|p==*x||p.starts_with(x)) || p.ends_with(".pem")||p.ends_with(".key")||p.to_lowercase().contains("credentials")||p.to_lowercase().contains("secrets") }
fn is_probably_binary_path(p:&str)->bool { let p=p.to_lowercase(); [".png",".jpg",".jpeg",".gif",".webp",".pdf",".zip",".gz",".exe",".dll",".so",".woff",".woff2",".ttf",".ico"].iter().any(|x|p.ends_with(x)) }
fn read_bounded(p:&Path,max:usize)->AppResult<String>{ let mut f=File::open(p)?; let mut b=Vec::new(); f.take(max as u64).read_to_end(&mut b)?; Ok(String::from_utf8_lossy(&b).into_owned()) }

fn openai_plan(c:&Config,item:&ProjectItem,ctx:&RepoContext)->AppResult<HandoffPlan>{
    if c.planning.provider!="openai" {return Err(AppError::new("PlannerError","only planning.provider=openai is supported"));}
    let key=env::var("OPENAI_API_KEY").map_err(|_|AppError::new("PlannerError","OPENAI_API_KEY is not set"))?;
    require_cmd("curl")?;
    let prompt=build_planner_prompt(c,item,ctx);
    let schema=r#"{"type":"object","additionalProperties":false,"required":["summary","objective","assumptions","relevant_files","proposed_changes","preparation","implementation_steps","validation_steps","risks","acceptance_criteria"],"properties":{"summary":{"type":"string"},"objective":{"type":"string"},"assumptions":{"type":"array","items":{"type":"string"}},"relevant_files":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["path","reason","expected_change"],"properties":{"path":{"type":"string"},"reason":{"type":"string"},"expected_change":{"type":["string","null"]}}}},"proposed_changes":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["file","description","risk"],"properties":{"file":{"type":["string","null"]},"description":{"type":"string"},"risk":{"type":["string","null"]}}}},"preparation":{"type":"array","items":{"type":"string"}},"implementation_steps":{"type":"array","items":{"type":"string"}},"validation_steps":{"type":"array","items":{"type":"string"}},"risks":{"type":"array","items":{"type":"string"}},"acceptance_criteria":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["criterion","validation"],"properties":{"criterion":{"type":"string"},"validation":{"type":["string","null"]}}}}}}"#;
    let body=format!(r#"{{"model":"{}","input":[{{"role":"system","content":[{{"type":"input_text","text":"You are a read-only senior software engineer planner. Inspect supplied repository context. Do not fabricate explicit acceptance criteria. Put only explicitly stated acceptance criteria in acceptance_criteria; implementation-derived checks belong in validation_steps. Produce a small, precise implementation plan."}}]}},{{"role":"user","content":[{{"type":"input_text","text":"{}"}}]}}],"text":{{"format":{{"type":"json_schema","name":"handoff_plan","strict":true,"schema":{}}}}}}}}"#,json_escape(&c.planning.model),json_escape(&prompt),schema);
    let tmp=temp_file("zoro-openai-request.json"); fs::write(&tmp,body)?;
    let out=Command::new("curl").args(["-sS","--fail-with-body","https://api.openai.com/v1/responses","-H",&format!("Authorization: Bearer {key}"),"-H","Content-Type: application/json","--data-binary",&format!("@{}",tmp.display())]).output().map_err(|e|AppError::new("PlannerError",e.to_string()))?;
    let _=fs::remove_file(&tmp);
    if !out.status.success(){return Err(AppError::new("PlannerError",String::from_utf8_lossy(&out.stderr).to_string()+&String::from_utf8_lossy(&out.stdout)));}
    let root=parse_json(&String::from_utf8_lossy(&out.stdout))?;
    let text=extract_openai_output_text(&root).ok_or_else(||AppError::new("PlannerError","OpenAI response contained no output_text"))?;
    parse_handoff_plan(&parse_json(&text)?)
}

fn build_planner_prompt(c:&Config,item:&ProjectItem,ctx:&RepoContext)->String{
    let mut s=format!("Repository: {}/{}\nProject item id: {}\nIssue: {}\nTitle: {}\n\nIssue body:\n{}\n\nGit status:\n{}\n\n",c.github.owner,c.github.repo,item.item_id,item.issue_number.map(|x|x.to_string()).unwrap_or("n/a".into()),item.title,item.body,if ctx.git_status.trim().is_empty(){"clean"}else{&ctx.git_status});
    for (p,t) in &ctx.instructions {s.push_str(&format!("\n--- repository instruction/metadata: {p} ---\n{t}\n"));}
    for (p,t) in &ctx.files {s.push_str(&format!("\n--- relevant file: {p} ---\n{t}\n"));}
    s
}
fn extract_openai_output_text(j:&Json)->Option<String>{
    if let Some(s)=j.get("output_text").and_then(Json::as_str){return Some(s.to_string());}
    for o in j.get("output")?.as_arr()? { for c in o.get("content")?.as_arr()? { if let Some(s)=c.get("text").and_then(Json::as_str){return Some(s.to_string());} } } None
}
fn parse_handoff_plan(j:&Json)->AppResult<HandoffPlan>{
    let gs=|k:&str| j.get(k).and_then(Json::as_str).map(str::to_string).ok_or_else(||AppError::new("PlannerError",format!("missing string {k}")));
    let arrs=|k:&str|->AppResult<Vec<String>>{Ok(j.get(k).and_then(Json::as_arr).ok_or_else(||AppError::new("PlannerError",format!("missing array {k}")))?.iter().filter_map(|x|x.as_str().map(str::to_string)).collect())};
    let mut rf=Vec::new(); for x in j.get("relevant_files").and_then(Json::as_arr).ok_or_else(||AppError::new("PlannerError","missing relevant_files"))? { rf.push(RelevantFile{path:x.get("path").and_then(Json::as_str).unwrap_or("").into(),reason:x.get("reason").and_then(Json::as_str).unwrap_or("").into(),expected_change:x.get("expected_change").and_then(Json::as_str).map(str::to_string)}); }
    let mut pc=Vec::new(); for x in j.get("proposed_changes").and_then(Json::as_arr).ok_or_else(||AppError::new("PlannerError","missing proposed_changes"))? {pc.push(ProposedChange{file:x.get("file").and_then(Json::as_str).map(str::to_string),description:x.get("description").and_then(Json::as_str).unwrap_or("").into(),risk:x.get("risk").and_then(Json::as_str).map(str::to_string)});}
    let mut ac=Vec::new(); for x in j.get("acceptance_criteria").and_then(Json::as_arr).ok_or_else(||AppError::new("PlannerError","missing acceptance_criteria"))? {ac.push(AcceptanceCriterion{criterion:x.get("criterion").and_then(Json::as_str).unwrap_or("").into(),validation:x.get("validation").and_then(Json::as_str).map(str::to_string)});}
    Ok(HandoffPlan{summary:gs("summary")?,objective:gs("objective")?,assumptions:arrs("assumptions")?,relevant_files:rf,proposed_changes:pc,preparation:arrs("preparation")?,implementation_steps:arrs("implementation_steps")?,validation_steps:arrs("validation_steps")?,risks:arrs("risks")?,acceptance_criteria:ac})
}

fn render_handoff(c:&Config,item:&ProjectItem,p:&HandoffPlan,ctx:&RepoContext)->AppResult<PathBuf>{
    let issue=item.issue_number.map(|x|x.to_string()).unwrap_or_else(||short_id(&item.item_id)); let slug=slugify(&item.title); let dir=Path::new(&c.handoff.directory).join("ready"); fs::create_dir_all(&dir)?; let path=dir.join(format!("{}-{}.md",issue,slug));
    let mut s=format!("---\nzoro_version: {}\nissue: {}\nrepository: {}/{}\nproject_item_id: {}\nstatus: ready\ngenerated_at: {}\nplanner: {}\nmodel: {}\n---\n\n# {}\n\n## Objective\n\n{}\n\n## Issue Context\n\n{}\n\n",VERSION,item.issue_number.map(|x|x.to_string()).unwrap_or("null".into()),c.github.owner,c.github.repo,item.item_id,iso_timestamp(),c.planning.provider,c.planning.model,item.title,p.objective,if item.body.trim().is_empty(){"No issue body was provided."}else{&item.body});
    s.push_str("## Acceptance Criteria\n\n"); if p.acceptance_criteria.is_empty(){s.push_str("No explicit acceptance criteria were present in the project item.\n");} else {for x in &p.acceptance_criteria{s.push_str(&format!("- [ ] {}{}\n",x.criterion,x.validation.as_ref().map(|v|format!(". Validation: {v}")).unwrap_or_default()));}}
    s.push_str("\n## Repository Analysis\n\n"); s.push_str(&p.summary); s.push_str("\n\n### Relevant Files\n\n"); for x in &p.relevant_files{s.push_str(&format!("- `{}`: {}{}\n",x.path,x.reason,x.expected_change.as_ref().map(|v|format!(" Expected change: {v}")).unwrap_or_default()));}
    section_list(&mut s,"Preparation",&p.preparation); s.push_str("\n## Proposed Changes\n\n"); for x in &p.proposed_changes{s.push_str(&format!("- {}{}{}\n",x.file.as_ref().map(|f|format!("`{f}`: ")).unwrap_or_default(),x.description,x.risk.as_ref().map(|r|format!(" Risk: {r}")).unwrap_or_default()));}
    section_num(&mut s,"Implementation Plan",&p.implementation_steps); section_list(&mut s,"Validation",&p.validation_steps); section_list(&mut s,"Risks",&p.risks);
    s.push_str("\n## Implementation Constraints\n\n- Follow repository instructions.\n- Inspect existing code before editing.\n- Implement only this handoff.\n- Do not refactor unrelated code.\n- Preserve developer changes.\n- Do not expose secrets.\n");
    if !ctx.git_status.trim().is_empty(){s.push_str("- Repository was dirty when this plan was generated. Implementation must not start until the working tree is clean.\n");}
    s.push_str("\n## Definition of Done\n\n- [ ] Requested implementation is complete.\n- [ ] Relevant tests have been added or updated where appropriate.\n- [ ] Configured validation commands pass, or missing validation is explicitly reported.\n- [ ] No unrelated changes were introduced.\n");
    fs::write(&path,s)?; Ok(path)
}
fn section_list(s:&mut String,title:&str,x:&[String]){s.push_str(&format!("\n## {title}\n\n")); if x.is_empty(){s.push_str("None identified.\n");}else{for a in x{s.push_str(&format!("- {a}\n"));}}}
fn section_num(s:&mut String,title:&str,x:&[String]){s.push_str(&format!("\n## {title}\n\n")); if x.is_empty(){s.push_str("None identified.\n");}else{for (i,a) in x.iter().enumerate(){s.push_str(&format!("{}. {a}\n",i+1));}}}

fn cmd_implement(issue:Option<u64>)->AppResult<()> { let c=load_config()?; let _lock=RepoLock::acquire()?; implement_item(&c,issue) }
fn implement_item(c:&Config,issue:Option<u64>)->AppResult<()> {
    if c.implementation.provider!="codex" {return Err(AppError::new("CodexError","only implementation.provider=codex is supported"));} require_cmd("codex")?; ensure_git_repo()?;
    if !git_clean()? {return Err(AppError::new("RepositoryError",format!("Cannot start implementation.\n\nRepository contains uncommitted changes:\n{}\nCommit, stash, or remove these changes first.",run_text("git",&["status","--porcelain"])?)));}
    let handoff=select_handoff(c,issue)?; let text=fs::read_to_string(&handoff)?; let number=frontmatter_value(&text,"issue").and_then(|x|x.parse::<u64>().ok()); let item_id=frontmatter_value(&text,"project_item_id").ok_or_else(||AppError::new("HandoffError","missing project_item_id"))?;
    let title=text.lines().find_map(|x|x.strip_prefix("# ")).unwrap_or("task").to_string();
    let implementing=move_handoff(c,&handoff,"implementing")?;
    let result=(||->AppResult<()> {
        let (_,meta)=load_project(c)?;
        if c.behavior.move_to_in_progress_on_implement {update_project_status(c,&meta,&item_id,&c.github.implementing)?;}
        if c.implementation.branch_enabled {create_branch(c,number,&title)?;}
        println!("• Invoking Codex"); run_codex(&implementing)?;
        if c.implementation.validation_enabled { run_validations(c)?; } else { println!("⚠ Validation disabled"); }
        let review=move_handoff(c,&implementing,"review")?;
        if c.behavior.move_to_review_on_success {update_project_status(c,&meta,&item_id,&c.github.review)?;}
        println!("✓ Implementation ready for review\n  {}",review.display()); Ok(())
    })();
    if let Err(e)=result { let failed=move_handoff(c,&implementing,"failed").unwrap_or(implementing); let mut f=OpenOptions::new().append(true).open(&failed).ok(); if let Some(ref mut file)=f {let _=writeln!(file,"\n## Implementation Failure\n\n- Time: {}\n- Error: {}\n",iso_timestamp(),e);} return Err(e); }
    Ok(())
}

fn select_handoff(c:&Config,issue:Option<u64>)->AppResult<PathBuf>{
    let dir=Path::new(&c.handoff.directory).join("ready"); let mut files:Vec<PathBuf>=fs::read_dir(&dir)?.filter_map(|e|e.ok().map(|x|x.path())).filter(|p|p.extension().and_then(|x|x.to_str())==Some("md")).collect(); files.sort();
    if files.is_empty(){return Err(AppError::new("HandoffError","no ready handoffs"));}
    if let Some(n)=issue { return files.into_iter().find(|p|p.file_name().and_then(|x|x.to_str()).map(|x|x.starts_with(&format!("{n}-"))).unwrap_or(false)).ok_or_else(||AppError::new("HandoffError",format!("no ready handoff for issue #{n}"))); }
    println!("Select a handoff to implement:\n"); for (i,p) in files.iter().enumerate(){println!("  {}. {}",i+1,p.file_name().unwrap().to_string_lossy());} print!("\nSelection: "); io::stdout().flush()?; let mut s=String::new(); io::stdin().read_line(&mut s)?; let i:usize=s.trim().parse().map_err(|_|AppError::new("CliError","invalid selection"))?; files.get(i.saturating_sub(1)).cloned().ok_or_else(||AppError::new("CliError","selection out of range"))
}
fn move_handoff(c:&Config,src:&Path,state:&str)->AppResult<PathBuf>{ if !src.exists(){return Ok(Path::new(&c.handoff.directory).join(state).join(src.file_name().unwrap_or_default()));} let dir=Path::new(&c.handoff.directory).join(state);fs::create_dir_all(&dir)?;let dst=dir.join(src.file_name().ok_or_else(||AppError::new("HandoffError","invalid filename"))?);fs::rename(src,&dst)?;Ok(dst)}
fn run_codex(handoff:&Path)->AppResult<()> { let prompt=format!("Follow repository instructions. Implement only the requested handoff at {}. Inspect existing code before editing. Do not refactor unrelated code. Run appropriate tests. Preserve user changes. Report affected files and validation.",handoff.display()); let status=Command::new("codex").args(["exec","--full-auto",&prompt]).status().map_err(|e|AppError::new("CodexError",e.to_string()))?; if !status.success(){return Err(AppError::new("CodexError",format!("Codex exited with {status}")));} Ok(()) }
fn run_validations(c:&Config)->AppResult<()> { if c.implementation.validation_commands.is_empty(){println!("⚠ No validation commands configured");return Ok(());} for cmd in &c.implementation.validation_commands {println!("• Validate: {cmd}"); let status=if cfg!(windows){Command::new("cmd").args(["/C",cmd]).status()}else{Command::new("sh").args(["-c",cmd]).status()}.map_err(|e|AppError::new("ValidationError",e.to_string()))?; if !status.success(){return Err(AppError::new("ValidationError",format!("failed: {cmd} ({status})")));}} println!("✓ Validation passed");Ok(()) }
fn create_branch(c:&Config,issue:Option<u64>,title:&str)->AppResult<()> { let n=issue.map(|x|x.to_string()).unwrap_or("item".into());let name=format!("{}/{}-{}",c.implementation.branch_prefix,n,slugify(title)); if Command::new("git").args(["show-ref","--verify","--quiet",&format!("refs/heads/{name}")]).status().map(|s|s.success()).unwrap_or(false){return Err(AppError::new("RepositoryError",format!("branch already exists: {name}")));} command_ok("git",&["switch","-c",&name])?;println!("✓ Branch {name}");Ok(()) }

fn cmd_run(once:bool)->AppResult<()> { let c=load_config()?; if once {let _lock=RepoLock::acquire()?;return run_once(&c);} if !c.scheduler.enabled{return Err(AppError::new("ConfigError","scheduler.enabled is false"));} let d=parse_duration(&c.scheduler.interval)?; println!("zoro run: polling every {}. Ctrl+C to stop.",c.scheduler.interval); loop { {match RepoLock::acquire(){Ok(_l)=>if let Err(e)=run_once(&c){eprintln!("⚠ {e}")},Err(e)=>eprintln!("⚠ {e}")}} thread::sleep(d); } }
fn run_once(c:&Config)->AppResult<()> { println!("• Checking project"); let (items,_)=load_project(c)?; let ready:Vec<_>=items.into_iter().filter(|x|x.status==c.github.ready).collect(); println!("• Found {} Ready items",ready.len()); let item=match ready.first(){Some(x)=>x,None=>return Ok(())}; if find_handoff(c,item.issue_number,&item.item_id,false)?.is_some(){println!("• Top Ready item already planned, skipping");return Ok(());} if !c.automation.auto_plan {println!("• auto_plan disabled");return Ok(());} let path=plan_item(c,item.issue_number)?; if c.automation.auto_implement {let issue=item.issue_number; println!("• auto_implement enabled"); implement_item(c,issue)?;} else {println!("• Waiting for developer: {}",path.display());} Ok(()) }

fn cmd_status()->AppResult<()> {let c=load_config()?;for s in ["ready","implementing","review","done","failed"]{let d=Path::new(&c.handoff.directory).join(s);let n=fs::read_dir(&d).map(|x|x.filter_map(Result::ok).filter(|e|e.path().extension().and_then(|x|x.to_str())==Some("md")).count()).unwrap_or(0);println!("{:<14} {}",s,n);}Ok(())}
fn cmd_config()->AppResult<()> { let c=load_config()?; println!("version: {}\ngithub: {}/{} project #{}\nstatus_field: {}\ninterval: {}\nplanner: {} / {}\nauto_plan: {}\nauto_implement: {}\nimplementation: {}\nbranch: {} / {}\nhandoff: {}\nvalidation commands: {}",c.version,c.github.owner,c.github.repo,c.github.project_number,c.github.status_field,c.scheduler.interval,c.planning.provider,c.planning.model,c.automation.auto_plan,c.automation.auto_implement,c.implementation.provider,c.implementation.branch_enabled,c.implementation.branch_prefix,c.handoff.directory,c.implementation.validation_commands.len()); Ok(()) }

struct RepoLock { path:PathBuf }
impl RepoLock { fn acquire()->AppResult<Self>{fs::create_dir_all(".zoro/runtime")?;match OpenOptions::new().write(true).create_new(true).open(LOCK_PATH){Ok(mut f)=>{writeln!(f,"pid={}\ntime={}",std::process::id(),iso_timestamp())?;Ok(Self{path:LOCK_PATH.into()})},Err(e) if e.kind()==io::ErrorKind::AlreadyExists=>Err(AppError::new("LockError",format!("another Zoro process holds {LOCK_PATH}"))),Err(e)=>Err(e.into())}}}
impl Drop for RepoLock {fn drop(&mut self){let _=fs::remove_file(&self.path);}}

fn load_project(c:&Config)->AppResult<(Vec<ProjectItem>,ProjectMeta)> {
    require_cmd("gh")?;
    let typ=gh_output(&["api",&format!("users/{}",c.github.owner),"--jq",".type"])?; let root=if typ.trim()=="Organization"{"organization"}else{"user"};
    let q=format!(r#"query {{ {root}(login: "{}") {{ projectV2(number: {}) {{ id fields(first: 100) {{ nodes {{ ... on ProjectV2SingleSelectField {{ id name options {{ id name }} }} }} }} items(first: 100) {{ nodes {{ id type content {{ ... on Issue {{ id number title body }} ... on PullRequest {{ id number title body }} ... on DraftIssue {{ title body }} }} fieldValues(first: 30) {{ nodes {{ ... on ProjectV2ItemFieldSingleSelectValue {{ name field {{ ... on ProjectV2SingleSelectField {{ id name }} }} }} }} }} }} }} }} }} }}"#,gql_escape(&c.github.owner),c.github.project_number);
    let raw=gh_graphql(&q)?; let j=parse_json(&raw)?; if let Some(errors)=j.get("errors").and_then(Json::as_arr){return Err(AppError::new("GitHubError",format!("GraphQL errors: {errors:?}")));}
    let project=j.get("data").and_then(|x|x.get(root)).and_then(|x|x.get("projectV2")).ok_or_else(||AppError::new("ProjectError",format!("could not resolve {root} '{}' project #{}",c.github.owner,c.github.project_number)))?;
    let project_id=project.get("id").and_then(Json::as_str).ok_or_else(||AppError::new("ProjectError","missing project id"))?.to_string();
    let fields=project.get("fields").and_then(|x|x.get("nodes")).and_then(Json::as_arr).ok_or_else(||AppError::new("ProjectError","missing fields"))?;
    let sf=fields.iter().find(|x|x.get("name").and_then(Json::as_str)==Some(c.github.status_field.as_str())).ok_or_else(||AppError::new("ProjectError",format!("status field '{}' not found",c.github.status_field)))?;
    let status_field_id=sf.get("id").and_then(Json::as_str).ok_or_else(||AppError::new("ProjectError","status field has no id"))?.to_string(); let mut options=HashMap::new(); for o in sf.get("options").and_then(Json::as_arr).unwrap_or(&[]) {if let (Some(id),Some(name))=(o.get("id").and_then(Json::as_str),o.get("name").and_then(Json::as_str)){options.insert(name.to_string(),id.to_string());}}
    for required in [&c.github.backlog,&c.github.ready,&c.github.implementing,&c.github.review,&c.github.done] {if !options.contains_key(required){return Err(AppError::new("ProjectError",format!("required status value '{required}' does not exist. Available: {}",options.keys().cloned().collect::<Vec<_>>().join(", "))));}}
    let nodes=project.get("items").and_then(|x|x.get("nodes")).and_then(Json::as_arr).ok_or_else(||AppError::new("ProjectError","missing project items"))?; let mut items=Vec::new();
    for (position,n) in nodes.iter().enumerate(){ let item_id=n.get("id").and_then(Json::as_str).unwrap_or("").to_string(); let content=n.get("content"); let title=content.and_then(|x|x.get("title")).and_then(Json::as_str).unwrap_or("Untitled").to_string(); let body=content.and_then(|x|x.get("body")).and_then(Json::as_str).unwrap_or("").to_string(); let issue_number=content.and_then(|x|x.get("number")).and_then(Json::as_u64); let issue_id=content.and_then(|x|x.get("id")).and_then(Json::as_str).map(str::to_string); let mut status=String::new(); if let Some(vals)=n.get("fieldValues").and_then(|x|x.get("nodes")).and_then(Json::as_arr){for v in vals{if v.get("field").and_then(|x|x.get("name")).and_then(Json::as_str)==Some(c.github.status_field.as_str()){status=v.get("name").and_then(Json::as_str).unwrap_or("").to_string();break;}}} items.push(ProjectItem{item_id,issue_id,issue_number,title,body,status,position}); }
    Ok((items,ProjectMeta{project_id,status_field_id,options}))
}
fn update_project_status(_c:&Config,m:&ProjectMeta,item_id:&str,status:&str)->AppResult<()> {let option=m.options.get(status).ok_or_else(||AppError::new("ProjectError",format!("unknown status: {status}")))?;let q=format!(r#"mutation {{ updateProjectV2ItemFieldValue(input: {{ projectId: "{}", itemId: "{}", fieldId: "{}", value: {{ singleSelectOptionId: "{}" }} }}) {{ projectV2Item {{ id }} }} }}"#,m.project_id,gql_escape(item_id),m.status_field_id,option); gh_graphql(&q)?;println!("✓ GitHub status -> {status}");Ok(()) }
fn gh_graphql(q:&str)->AppResult<String>{gh_output(&["api","graphql","-f",&format!("query={q}")])}
fn gh_output(args:&[&str])->AppResult<String>{let mut cmd=Command::new("gh");cmd.args(args); if let Some(t)=resolve_github_token(){cmd.env("GH_TOKEN",t);}let out=cmd.output().map_err(|e|AppError::new("GitHubError",e.to_string()))?;if !out.status.success(){return Err(AppError::new("GitHubError",String::from_utf8_lossy(&out.stderr).trim().to_string()));}Ok(String::from_utf8_lossy(&out.stdout).to_string())}
fn resolve_github_token()->Option<String>{env::var("ZORO_GITHUB_TOKEN").ok().filter(|x|!x.is_empty()).or_else(||env::var("GH_TOKEN").ok().filter(|x|!x.is_empty()))}
fn gql_escape(s:&str)->String{s.replace('\\',"\\\\").replace('"',"\\\"")}

fn find_handoff(c:&Config,issue:Option<u64>,item_id:&str,include_failed:bool)->AppResult<Option<PathBuf>> {let mut states=vec!["ready","implementing","review","done"];if include_failed{states.push("failed");}for st in states{let d=Path::new(&c.handoff.directory).join(st);if !d.exists(){continue;}for e in fs::read_dir(d)?{let p=e?.path();if p.extension().and_then(|x|x.to_str())!=Some("md"){continue;}let filename=p.file_name().unwrap_or_default().to_string_lossy();if issue.map(|n|filename.starts_with(&format!("{n}-"))).unwrap_or(false){return Ok(Some(p));}if let Ok(t)=fs::read_to_string(&p){if frontmatter_value(&t,"project_item_id").as_deref()==Some(item_id){return Ok(Some(p));}}}}Ok(None)}
fn frontmatter_value(s:&str,key:&str)->Option<String>{let mut lines=s.lines();if lines.next()? != "---"{return None;}for l in lines{if l=="---"{break;}if let Some((k,v))=l.split_once(':'){if k.trim()==key{return Some(v.trim().trim_matches('"').to_string());}}}None}

fn infer_github_repo()->AppResult<(String,String)>{let u=run_text("git",&["remote","get-url","origin"])?;let u=u.trim().trim_end_matches(".git");if let Some(x)=u.strip_prefix("git@github.com:"){return split_owner_repo(x);}if let Some(x)=u.strip_prefix("https://github.com/"){return split_owner_repo(x);}if let Some(x)=u.strip_prefix("http://github.com/"){return split_owner_repo(x);}Err(AppError::new("RepositoryError",format!("unsupported GitHub origin: {u}")))}
fn split_owner_repo(s:&str)->AppResult<(String,String)>{let mut x=s.split('/');let a=x.next().unwrap_or("");let b=x.next().unwrap_or("");if a.is_empty()||b.is_empty(){Err(AppError::new("RepositoryError","cannot infer owner/repository"))}else{Ok((a.into(),b.into()))}}
fn ensure_git_repo()->AppResult<()> {if run_text("git",&["rev-parse","--is-inside-work-tree"]).map(|x|x.trim()=="true").unwrap_or(false){Ok(())}else{Err(AppError::new("RepositoryError","not inside a Git repository"))}}
fn git_clean()->AppResult<bool>{Ok(run_text("git",&["status","--porcelain"])?.trim().is_empty())}
fn add_gitignore_line(line:&str)->AppResult<()> {let p=Path::new(".gitignore");let old=fs::read_to_string(p).unwrap_or_default();if old.lines().any(|x|x.trim()==line){return Ok(());}let mut f=OpenOptions::new().create(true).append(true).open(p)?;if !old.is_empty()&&!old.ends_with('\n'){writeln!(f)?;}writeln!(f,"{line}")?;Ok(())}

fn command_ok(cmd:&str,args:&[&str])->AppResult<()> {let s=Command::new(cmd).args(args).status().map_err(|e|AppError::new("CommandError",format!("{cmd}: {e}")))?;if !s.success(){Err(AppError::new("CommandError",format!("{cmd} exited with {s}")))}else{Ok(())}}
fn run_text(cmd:&str,args:&[&str])->AppResult<String>{let o=Command::new(cmd).args(args).output().map_err(|e|AppError::new("CommandError",format!("{cmd}: {e}")))?;if !o.status.success(){Err(AppError::new("CommandError",String::from_utf8_lossy(&o.stderr).trim().to_string()))}else{Ok(String::from_utf8_lossy(&o.stdout).to_string())}}
fn cmd_exists(cmd:&str)->bool{Command::new(cmd).arg("--version").stdout(Stdio::null()).stderr(Stdio::null()).status().map(|s|s.success()).unwrap_or(false)}
fn require_cmd(cmd:&str)->AppResult<()> {if cmd_exists(cmd){Ok(())}else{Err(AppError::new("DependencyError",format!("required executable not found in PATH: {cmd}")))}}
fn display_item(i:&ProjectItem)->String{match i.issue_number{Some(n)=>format!("#{n} {}",i.title),None=>i.title.clone()}}
fn slugify(s:&str)->String{let mut o=String::new();let mut dash=false;for c in s.to_lowercase().chars(){if c.is_ascii_alphanumeric(){o.push(c);dash=false;}else if !dash&&!o.is_empty(){o.push('-');dash=true;}}while o.ends_with('-'){o.pop();}if o.len()>64{o.truncate(64);while o.ends_with('-'){o.pop();}}if o.is_empty(){"task".into()}else{o}}
fn short_id(s:&str)->String{s.chars().rev().take(8).collect::<String>().chars().rev().collect()}
fn temp_file(name:&str)->PathBuf{env::temp_dir().join(format!("{}-{}-{}",std::process::id(),now_secs(),name))}
fn now_secs()->u64{SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default().as_secs()}
fn iso_timestamp()->String{
    // UTC ISO-8601 without external time crate. Deterministic and standards-compliant.
    let secs=now_secs() as i64; let days=secs.div_euclid(86400); let rem=secs.rem_euclid(86400); let (y,m,d)=civil_from_days(days);format!("{:04}-{:02}-{:02}T{:02}:{:02}:{:02}Z",y,m,d,rem/3600,(rem%3600)/60,rem%60)
}
fn civil_from_days(z:i64)->(i64,i64,i64){let z=z+719468;let era=if z>=0{z}else{z-146096}/146097;let doe=z-era*146097;let yoe=(doe-doe/1460+doe/36524-doe/146096)/365;let y=yoe+era*400;let doy=doe-(365*yoe+yoe/4-yoe/100);let mp=(5*doy+2)/153;let d=doy-(153*mp+2)/5+1;let m=mp+if mp<10{3}else{-9};(y+if m<=2{1}else{0},m,d)}
