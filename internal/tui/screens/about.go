package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noisethanks/atak/internal/tui/style"
)

const licenseConst = `Third-Party Licenses
====================


github.com/matyalatte/Texconv-Custom-DLL
-----------------------------------------
MIT License
Copyright (c) 2022 matyalatte
https://github.com/matyalatte/Texconv-Custom-DLL

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


Texconv-Custom-DLL Dependencies
---------------------------------
Third-Party Licenses

This software includes third-party libraries.
This software is based in part on the work of the Independent JPEG Group.
The following licenses apply to the respective components.

MIT License

The following projects are licensed under the MIT License:

- DirectXTex
  Copyright (c) Microsoft Corporation.
  URL: https://github.com/microsoft/DirectXTex

- DirectX-Headers
  Copyright (c) Microsoft Corporation.
  URL: https://github.com/microsoft/DirectX-Headers

- DirectXMath
  Copyright (c) Microsoft Corporation.
  URL: https://github.com/microsoft/DirectXMath

- safestringlib (only included in Linux/macOS binaries)
  Copyright (c) 2014-2018 Intel Corporation
  URL: https://github.com/intel/safestringlib

- libdeflate
  Copyright 2016 Eric Biggers
  Copyright 2024 Google LLC
  URL: https://github.com/ebiggers/libdeflate

----- MIT License Text -----

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


zlib/libpng License

The following projects are licensed under the zlib/libpng License:

- libpng (only included in Linux/macOS binaries)
  * Copyright (c) 1995-2025 The PNG Reference Library Authors.
  * Copyright (c) 2018-2025 Cosmin Truta.
  * Copyright (c) 2000-2002, 2004, 2006-2018 Glenn Randers-Pehrson.
  * Copyright (c) 1996-1997 Andreas Dilger.
  * Copyright (c) 1995-1996 Guy Eric Schalnat, Group 42, Inc.
  URL: https://github.com/pnggroup/libpng

- zlib (only included in Linux binaries)
  (C) 1995-2025 Jean-loup Gailly and Mark Adler
  URL: https://github.com/madler/zlib.git

- SIMD extension for libjpeg-turbo (only included in Linux/macOS binaries)
  Copyright (C) 1999-2006, MIYASAKA Masaru.
  and more libjpeg-turbo contributors. (Individual contributors are listed in the source files)
  URL: https://github.com/libjpeg-turbo/libjpeg-turbo

----- zlib/libpng License Text -----

The software is supplied "as is", without warranty of any kind,
express or implied, including, without limitation, the warranties
of merchantability, fitness for a particular purpose, title, and
non-infringement.  In no event shall the Copyright owners, or
anyone distributing the software, be liable for any damages or
other liability, whether in contract, tort or otherwise, arising
from, out of, or in connection with the software, or the use or
other dealings in the software, even if advised of the possibility
of such damage.

Permission is hereby granted to use, copy, modify, and distribute
this software, or portions hereof, for any purpose, without fee,
subject to the following restrictions:

 1. The origin of this software must not be misrepresented; you
    must not claim that you wrote the original software.  If you
    use this software in a product, an acknowledgment in the product
    documentation would be appreciated, but is not required.

 2. Altered source versions must be plainly marked as such, and must
    not be misrepresented as being the original software.

 3. This Copyright notice may not be removed or altered from any
    source or altered source distribution.


IJG (Independent JPEG Group) License

The following project is licensed under the zlib/libpng License:

- libjpeg API library for libjpeg-turbo (only included in Linux/macOS binaries)
  URL: https://github.com/libjpeg-turbo/libjpeg-turbo

----- IJG License Text -----

The authors make NO WARRANTY or representation, either express or implied,
with respect to this software, its quality, accuracy, merchantability, or
fitness for a particular purpose.  This software is provided "AS IS", and you,
its user, assume the entire risk as to its quality and accuracy.

This software is copyright (C) 1991-2020, Thomas G. Lane, Guido Vollbeding.
All Rights Reserved except as specified below.

Permission is hereby granted to use, copy, modify, and distribute this
software (or portions thereof) for any purpose, without fee, subject to these
conditions:
(1) If any part of the source code for this software is distributed, then this
README file must be included, with this copyright and no-warranty notice
unaltered; and any additions, deletions, or changes to the original files
must be clearly indicated in accompanying documentation.
(2) If only executable code is distributed, then the accompanying
documentation must state that "this software is based in part on the work of
the Independent JPEG Group".
(3) Permission for use of this software is granted only if the user accepts
full responsibility for any undesirable consequences; the authors accept
NO LIABILITY for damages of any kind.

These conditions apply to any software derived from or based on the IJG code,
not just to the unmodified library.  If you use our work, you ought to
acknowledge us.

Permission is NOT granted for the use of any IJG author's name or company name
in advertising or publicity relating to this software or products derived from
it.  This software may be referred to only as "the Independent JPEG Group's
software".

We specifically permit and encourage the use of this software as the basis of
commercial products, provided that all warranty or liability claims are
assumed by the product vendor.


BSD-3-Clause License

The following projects are licensed under the BSD-3-Clause License:

- OpenEXR
  Copyright (c) Contributors to the OpenEXR Project. All rights reserved.
  URL: https://github.com/AcademySoftwareFoundation/openexr

- Imath
  Copyright Contributors to the OpenEXR Project. All rights reserved.
  URL: https://github.com/AcademySoftwareFoundation/Imath

----- BSD-3-Clause License Text -----

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its contributors
   may be used to endorse or promote products derived from this software
   without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

BSD-2-Clause License

The following project is licensed under the BSD2-Clause License:

- OpenJPH
  Copyright (c) 2019, Aous Naman
  Copyright (c) 2019, Kakadu Software Pty Ltd, Australia
  Copyright (c) 2019, The University of New South Wales, Sydney, Australia
  URL: https://github.com/aous72/OpenJPH

----- BSD-2-Clause License Text -----

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.


7-Zip
------
GNU Lesser General Public License v2.1
Copyright (C) 1999-2023 Igor Pavlov
https://www.7-zip.org

7-Zip is free software. You can use 7-Zip on any computer, including a
computer in a commercial organization. You don't need to register or pay
for 7-Zip.

This binary includes 7zz, distributed under the GNU LGPL v2.1. The source
code is available at https://www.7-zip.org/download.html

Full license text: https://www.gnu.org/licenses/old-licenses/lgpl-2.1.html


github.com/charmbracelet/bubbletea
------------------------------------
MIT License
Copyright (c) 2020-present Charmbracelet, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


github.com/charmbracelet/bubbles
----------------------------------
MIT License
Copyright (c) 2020-present Charmbracelet, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


github.com/charmbracelet/lipgloss
-----------------------------------
MIT License
Copyright (c) 2021-present Charmbracelet, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


github.com/sahilm/fuzzy
------------------------
MIT License
Copyright (c) 2017 Sahil Muthoo

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


compressonator-bc7e (optional compression backend)
------------------------------------------------------------------------
This backend is a fork of AMD Compressonator with the CPU-side BC7 codec
replaced by bc7e.ispc from richgel999/bc7enc_rdo. Two licenses apply.

--- AMD Compressonator (MIT) ---
Copyright (c) 2024 Advanced Micro Devices, Inc. All rights reserved.
Copyright (c) 2004-2006 ATI Technologies Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.

--- nlohmann/json (MIT, linked into Compressonator) ---
Copyright (c) 2013-2017 Niels Lohmann

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

--- bc7e.ispc (Apache License 2.0) ---
This software incorporates bc7e.ispc from richgel999/bc7enc_rdo,
Copyright (C) 2018-2021 Binomial LLC, licensed under the Apache License,
Version 2.0. The bc7enc_rdo project is © Richard Geldreich, Jr.

Attribution notice (required by Apache License 2.0, §4d):

  This software incorporates bc7e.ispc from richgel999/bc7enc_rdo,
  © Richard Geldreich / Binomial LLC, licensed under the Apache License,
  Version 2.0.

The bc7enc_rdo repository LICENSE file (included verbatim below) identifies
bc7e.ispc as Apache 2.0 and provides the copyright. The full Apache 2.0
license text follows.

--- bc7enc_rdo LICENSE (as shipped by the upstream repo) ---
If you use this software in a product, attribution / credits is requested
but not required.

bc7e.ispc uses the Apache 2.0 license and is Copyright (C) 2018-2021
Binomial LLC.

All other source code files in that repo are available under either the MIT
License (Copyright (c) 2020-2021 Richard Geldreich, Jr.) or the Unlicense
public-domain dedication — atak links only bc7e.ispc, so the Apache 2.0
grant is the operative one for this distribution.

--- Apache License, Version 2.0 (full text) ---
` + apacheLicense2_0

// AboutModel displays version info and scrollable third-party license text.
type AboutModel struct {
	version string
	lines   []string
	offset  int
	width   int
	height  int
}

// licenseText param kept for API compatibility; hardcoded licenseConst is used instead.
func NewAbout(version string, licenseText []byte) AboutModel {
	return AboutModel{
		version: version,
		lines:   strings.Split(licenseConst, "\n"),
	}
}

func (m AboutModel) Init() tea.Cmd { return nil }

func (m AboutModel) Update(msg tea.Msg) (AboutModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "down", "j":
			max := len(m.lines) - m.visibleLines()
			if max < 0 {
				max = 0
			}
			if m.offset < max {
				m.offset++
			}
		case "up", "k":
			if m.offset > 0 {
				m.offset--
			}
		case "q", "esc":
			return m, func() tea.Msg { return NavigateMsg{To: NavMenu} }
		}
	}
	return m, nil
}

func (m AboutModel) visibleLines() int {
	v := m.height - 10
	if v < 1 {
		return 1
	}
	return v
}

func (m AboutModel) View() string {
	var b strings.Builder
	b.WriteString(style.StyleTitle.Render("atak "+m.version) + "\n\n")
	b.WriteString(style.StyleBody.Render("A texture compression and backup utility for") + "\n")
	b.WriteString(style.StyleBody.Render("S.T.A.L.K.E.R. Anomaly modlists.") + "\n\n")
	b.WriteString(style.StyleMuted.Render("github.com/noisethanks/atak") + "\n\n\n")

	visible := m.visibleLines()
	end := m.offset + visible
	if end > len(m.lines) {
		end = len(m.lines)
	}
	for _, line := range m.lines[m.offset:end] {
		b.WriteString(style.StyleBody.Render(line) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(style.KeyHint("↑↓", "scroll") + "  " + style.KeyHint("q/esc", "back"))
	return b.String()
}

func (m *AboutModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}
