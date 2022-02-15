<!--

    Copyright (c) 2021-present Sonatype, Inc.

    Licensed under the Apache License, Version 2.0 (the "License");
    you may not use this file except in compliance with the License.
    You may obtain a copy of the License at

        http://www.apache.org/licenses/LICENSE-2.0

    Unless required by applicable law or agreed to in writing, software
    distributed under the License is distributed on an "AS IS" BASIS,
    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    See the License for the specific language governing permissions and
    limitations under the License.

-->
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
