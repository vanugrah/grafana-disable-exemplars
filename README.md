# grafana-disable-exemplars
Scripts to find dashboards that have exemplar queries  and update them to disable exemplars. This is largely because Grafana had enabled exemplar queries by default for several verisions and only disabled it by default in version 8.5. Exmplar queries can add latency to overall query reponse times. This script disables exemplar queries across all dashboards and panels. 


# Usage 
```
go build
```

```
./grafana-disable-exemplars -url "https://www.your-grafana-instance.com/" -api-token "super-secure-api-token"
```

# Notes 
- The first step is to incrementally search through all dashboard models for uses of exemplars and saving dashboard UID's to a file. 
- The second step is to read the file from the first step and incrementally update the dashboard model and save the dashbaord. 
- If there are any failed transactions, a new file will be created with the dashboard UIDS of the failed transactions. 

# Todo
- Split out searching for dashboards with exemplars and removing exemplars into subcommands. 

