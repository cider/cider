# What to Fix or Improve

* Improve logging coverage (do not just silently ignore errors).
* Somehow collect significant environmental variables to be used in help.
* Rewrite the exchanges to use channels instead of mutexes.
* Make the interfaces a bit more friendly for inproc communication, e.g. values
  should be passed around in decoded way, not encoded. Well, this needs some
  thinking, either inproc or inter-process communication will be slower.
