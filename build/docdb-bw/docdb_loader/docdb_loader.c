#include <dlfcn.h>
#include "postgres.h"
#include "pgstat.h"
#include "postmaster/bgworker.h"
#include "storage/ipc.h"
#include "storage/latch.h"
#include "storage/proc.h"
#include "miscadmin.h"
#include "utils/elog.h"
#include "fmgr.h"

PG_MODULE_MAGIC;

static char *docdb_so = "docdb.so";

PGDLLEXPORT void background_main(Datum main_arg);

void background_main(Datum main_arg)
{
    bgworker_main_type f;
    char *path;
    void *h;

    BackgroundWorkerUnblockSignals();

    path = (char *)palloc(strlen(pkglib_path) + 1 + strlen(docdb_so) + 1);
    join_path_components(path, pkglib_path, docdb_so);
    elog(LOG, "docdb_loader: loading '%s'", path);

    h = dlopen(path, RTLD_NOW | RTLD_GLOBAL);
    pfree(path);

    f = (bgworker_main_type)dlsym(h, "BackgroundWorkerMain");
    f(main_arg);

    dlclose(h);
    proc_exit(0);
}

void _PG_init(void)
{
    BackgroundWorker worker;
    MemSet(&worker, 0, sizeof(BackgroundWorker));

    snprintf(worker.bgw_name, BGW_MAXLEN, "DocDBLoader");
    worker.bgw_flags = BGWORKER_SHMEM_ACCESS;
    worker.bgw_start_time = BgWorkerStart_RecoveryFinished;
    worker.bgw_restart_time = BGW_NEVER_RESTART;
    snprintf(worker.bgw_library_name, BGW_MAXLEN, "docdb_loader");
    snprintf(worker.bgw_function_name, BGW_MAXLEN, "background_main");

    RegisterBackgroundWorker(&worker);
}
