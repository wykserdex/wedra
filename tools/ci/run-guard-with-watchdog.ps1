# run-guard-with-watchdog.ps1 — внешняя сторожка для windows-test-guard.ps1.
#
# Идея из 7d1ad20 (сторожка снаружи, а не дедлайн внутри того, что может встать)
# сохранена. Что изменено и почему:
#
#  * сторожка запускает рабочий скрипт с редиректом в ФАЙЛЫ, а не через
#    Start-Process -RedirectStandardOutput/-RedirectStandardError. У Start-Process
#    это .NET-пайпы, а Process.WaitForExit() БЕЗ аргумента (в отличие от
#    WaitForExit(ms)) ждёт не только выхода процесса, но и EOF этих пайпов.
#    Значит .NET-слой сам может стать заблокированным читателем уже ПОСЛЕ того,
#    как всё отработало: ветка таймаута (единственная, где печатаются улики) не
#    наступает, потому что с точки зрения сторожки всё успешно, а шаг стоит до
#    отмены джоба и теряет лог. Файловый хендл, который кто-то держит, не
#    блокирует никого.
#  * по истечении бюджета печатается не только хвост логов, но и ТАБЛИЦА
#    ПРОЦЕССОВ с родителями: она отвечает на вопрос «какой процесс переживает
#    тест», а стопка текстовых логов — нет.
#  * у всех ожиданий внутри есть предел.
#
# Всё напечатанное дублируется в GITHUB_STEP_SUMMARY: он переживает отмену
# джоба, в отличие от лога шага и артефакта.

[CmdletBinding()]
param(
    [int]$BudgetSec = 1200,
    [int]$WorkerBudgetSec = -1,   # бюджет самого рабочего скрипта (по умолчанию BudgetSec-60)
    [int]$PerPackageSec = 660,
    [int]$GoTimeoutSec = 600,
    [switch]$KillLeakedDescendants,
    [string]$LogDir = (Join-Path $env:RUNNER_TEMP 'gotest'),
    [string]$GuardPath = '',
    [string]$StepSummary = $env:GITHUB_STEP_SUMMARY
)

$ErrorActionPreference = 'Continue'
$script:IsWin = ($env:OS -eq 'Windows_NT')

# $PSScriptRoot в значениях по умолчанию param() под Windows PowerShell 5.1
# пуст (там ещё не выставлен), поэтому путь к воркеру ищем в теле скрипта.
# Проверено: с `-File` значение по умолчанию давало Join-Path с пустым Path.
if (-not $GuardPath) {
    $here = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
    $GuardPath = Join-Path $here 'windows-test-guard.ps1'
}
if (-not (Test-Path -LiteralPath $GuardPath)) {
    throw "рабочий скрипт не найден: $GuardPath"
}

function Write-Both {
    param([string]$Text)
    Write-Host $Text
    if ($StepSummary) { Add-Content -Path $StepSummary -Value $Text -ErrorAction SilentlyContinue }
}

function Kill-Tree {
    param([int]$TargetPid)
    if ($script:IsWin) {
        & "$env:SystemRoot\System32\taskkill.exe" /T /F /PID $TargetPid 2>&1 | Out-Null
        return
    }
    $kids = @(ps -eo 'pid=,ppid=' | Where-Object { $_ -match '^\s*(\d+)\s+(\d+)$' -and [int]$Matches[2] -eq $TargetPid } |
        ForEach-Object { [int]($_-replace '^\s*(\d+).*', '$1') })
    foreach ($k in $kids) { Kill-Tree -TargetPid $k }
    & kill -9 $TargetPid 2>$null | Out-Null
}

function Get-CensusText {
    if ($script:IsWin) {
        try {
            return (Get-CimInstance Win32_Process -OperationTimeoutSec 10 -ErrorAction Stop |
                Sort-Object ProcessId |
                ForEach-Object { "pid={0} ppid={1} {2} :: {3}" -f $_.ProcessId, $_.ParentProcessId, $_.Name, ($_.CommandLine -replace '\s+', ' ') })
        } catch {
            return (Get-Process -ErrorAction SilentlyContinue |
                ForEach-Object { "pid={0} {1}" -f $_.Id, $_.ProcessName })
        }
    }
    return (& ps -eo 'pid=,ppid=,etimes=,args=')
}

function Dump-Evidence {
    param([string]$LogDir, [string]$Out, [string]$Err)
    Write-Both "--- census на момент зависания (кто вообще есть, с родителями) ---"
    foreach ($line in (Get-CensusText)) { Write-Both ("    " + $line) }
    foreach ($f in @($Out, $Err)) {
        Write-Both ("--- хвост {0} ---" -f (Split-Path $f -Leaf))
        if (Test-Path $f) { Get-Content $f | Select-Object -Last 60 | ForEach-Object { Write-Both ("    " + $_) } }
    }
    Write-Both "--- пер-пакетные логи (последние 15 строк каждого) ---"
    Get-ChildItem $LogDir -Filter '*.txt' -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -notin @((Split-Path $Out -Leaf), (Split-Path $Err -Leaf)) } |
        ForEach-Object {
            Write-Both ("### {0} ({1} байт, изменён {2})" -f $_.Name, $_.Length, $_.LastWriteTime.ToString('HH:mm:ss'))
            Get-Content $_.FullName -Tail 15 -ErrorAction SilentlyContinue | ForEach-Object { Write-Both ("    " + $_) }
        }
}

if ($WorkerBudgetSec -lt 0) { $WorkerBudgetSec = [Math]::Max(30, $BudgetSec - 60) }
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
$out = Join-Path $LogDir 'guard.out.txt'
$err = Join-Path $LogDir 'guard.err.txt'

$hostExe = (Get-Process -Id $PID).Path
$killArg = if ($KillLeakedDescendants) { ' -KillLeakedDescendants' } else { '' }
Write-Both ("watchdog: host=$hostExe budget=${BudgetSec}s workerBudget=${WorkerBudgetSec}s guard=$GuardPath")

# Рабочий скрипт — отдельный процесс, вывод уходит в ФАЙЛЫ (никаких пайпов).
$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.UseShellExecute = $false
$psi.RedirectStandardOutput = $false
$psi.RedirectStandardError = $false
if ($script:IsWin) {
    # Редирект ОБЯЗАН идти через cmd.exe. Проверено на этой машине: при
    # FileName = pwsh.exe и `> out` прямо в Arguments оператор редиректа НЕ
    # выполняется — он не является частью синтаксиса argv, молча
    # проглатывается, вывод уходит в унаследованный stdout родителя, а
    # выход равен 0. Т.е. заявленное в шапке «пайпов нет» оказывалось бы
    # неправдой, а Dump-Evidence читал бы несуществующие файлы.
    $psi.FileName = $env:ComSpec
    $psi.Arguments = '/c ""{0}" -NoProfile -ExecutionPolicy Bypass -File "{1}" -LogDir "{2}" -BudgetSec {3} -PerPackageSec {4} -GoTimeoutSec {5}{6} > "{7}" 2> "{8}""' -f `
        $hostExe, $GuardPath, $LogDir, $WorkerBudgetSec, $PerPackageSec, $GoTimeoutSec, $killArg, $out, $err
} else {
    # Ветка только для прогона сторожки вне Windows (проверка логики).
    $psi.FileName = '/bin/sh'
    $psi.Arguments = "-c ""exec '{0}' -NoProfile -File '{1}' -LogDir '{2}' -BudgetSec {3} -PerPackageSec {4} -GoTimeoutSec {5}{6} > '{7}' 2> '{8}'""" -f `
        $hostExe, $GuardPath, $LogDir, $WorkerBudgetSec, $PerPackageSec, $GoTimeoutSec, $killArg, $out, $err
}
# Handle кэшируется до чтения ExitCode: без этого свойство может быть пустым
# (та же причина, что и у Start-Process -PassThru в 10d78eb).
$worker = [System.Diagnostics.Process]::Start($psi)
$null = $worker.Handle
Write-Both ("watchdog: worker pid={0}" -f $worker.Id)

if (-not $worker.WaitForExit($BudgetSec * 1000)) {
    Write-Both ("watchdog: worker не уложился в {0} с — снимаю улики и убиваю дерево" -f $BudgetSec)
    Dump-Evidence -LogDir $LogDir -Out $out -Err $err
    Kill-Tree -TargetPid $worker.Id
    [void]$worker.WaitForExit(20000)
    if (-not $worker.HasExited) { Write-Both "watchdog: дерево worker'а всё ещё живо после taskkill" }
    exit 1
}
# Успешная ветка: читаем ФАЙЛЫ. Пайпов у нас нет вообще, поэтому ждать EOF тут
# нечего: ни WaitForExit(ms), ни ExitCode не могут быть заблокированы чужим
# потомком, который удержал бы перенаправленный поток.
if (Test-Path $out) { Get-Content $out | ForEach-Object { Write-Host $_ } }
if (Test-Path $err) { Get-Content $err | ForEach-Object { Write-Host $_ } }
if ($worker.ExitCode -ne 0) { Write-Both ("watchdog: worker exit={0}" -f $worker.ExitCode); exit 1 }
exit 0
