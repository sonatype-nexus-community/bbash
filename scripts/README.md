python script stuff
===================

Setup .venv, poetry? etc.

1. virtual environment
```bash
cd scripts
python3 -m venv ./.venv
source .venv/bin/activate
```
2. ??(not needed yet) install poetry (if not already installed)
```bash
curl -sSL https://raw.githubusercontent.com/python-poetry/poetry/master/install-poetry.py | python -
export PATH="/Users/bhamail/Library/Python/3.9/bin:$PATH"
poetry --version
```
3. install `requirements.txt`
```bash
pip install -r requirements.txt
```
4. run the script to add participants
```bash
python add-participants.py partslist.csv
```
`partslist.csv` file must have first line containing these headings:
```bash
GitHub ID,Email
```
Subsequent lines should contain a participant githubId and email address
